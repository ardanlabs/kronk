# Chapter 5: Message Caching

## Table of Contents

- [5.1 What IMC Does](#51-what-imc-does)
  - [5.1.1 Quick Semantic Understanding](#511-quick-semantic-understanding)
- [5.2 How Kronk Reuses a Text Prefix](#52-how-kronk-reuses-a-text-prefix)
- [5.3 Sessions, Slots, and Snapshots](#53-sessions-slots-and-snapshots)
- [5.4 Media Requests](#54-media-requests)
- [5.5 Configuration and Storage](#55-configuration-and-storage)
- [5.6 Invalidation and Limitations](#56-invalidation-and-limitations)
- [5.7 Observability](#57-observability)

---

## 5.1 What IMC Does

Incremental Message Cache (IMC) reduces repeated prompt processing in
multi-turn conversations. Without IMC, the model must prefill the complete
conversation before generating each response. With IMC, Kronk can restore a
previously processed prompt prefix and prefill only the new portion.

IMC is enabled by default for generation models. It is most useful for:

- Long-running chat and coding-agent conversations
- Tool-calling workflows that append results to the existing history
- Multiple agents or conversation branches sharing one model
- Prompts with expensive media that remains unchanged across follow-up turns

Short, one-shot prompts generally gain little from caching. IMC also performs
host-side rendering, tokenization, snapshot, and restore work, so it is not a
replacement for choosing an appropriate context window and concurrency level.

### 5.1.1 Quick Semantic Understanding

IMC is best understood as **prompt planning backed by reusable model state**.
Before assigning a request to an execution slot, Kronk determines the exact
rendered prompt, its reusable stable portion, the inference-only tail, the
compatible saved state, and the work required to move from that state to the
current request.

This is prompt-oriented rather than message-oriented because the model does not
consume message objects directly. It consumes rendered tokens, media
embeddings, and positions. Requests with apparently unchanged messages can
render differently when tools, thinking settings, templates, media, or other
render-affecting inputs change.

The planning process has five steps.

#### Step 1: Render the complete prompt with and without the generation suffix

Kronk independently renders the complete conversation in two forms:

```text
Stable rendering:           [complete conversation]
Generation-ready rendering: [complete conversation][generation suffix]
```

The stable rendering disables the generation prompt and becomes the reusable
cache target. The generation-ready rendering is the prompt used for inference.
Its suffix is normally template material that begins the next assistant turn,
such as an assistant header or control tokens; it is not the answer generated
by the model.

The stable rendering must be an exact prefix of the generation-ready rendering,
and the generation-ready rendering must contribute a nonempty tail. Kronk
renders both complete prompts instead of independently rendering a suffix
because a chat template can make separators, control tokens, and role formatting
depend on the surrounding messages. If the two renderings are not
prefix-compatible, Kronk safely processes the complete generation-ready prompt
without IMC reuse.

#### Step 2: Build a canonical token or media plan

For text, the canonical plan is the complete sequence of rendered token IDs:

```text
Stable plan: [A B C D]
Actual plan: [A B C D][G]
```

`[G]` is the generation-ready tail. It is processed for inference but is not
part of the reusable stable snapshot.

For media, token equality alone is insufficient. Kronk builds an ordered
logical plan containing text tokens and media-content identities:

```text
[token][token][image digest][token][audio digest][token]
```

This plan captures media content, count, order, and placement relative to text.
It is called canonical because it is the authoritative logical description of
what the engine must execute, not merely a normalized version of the client's
messages.

A media item is one logical plan unit, but the multimodal pipeline may expand it
into many physical KV cells. Models using M-RoPE can also distinguish the next
logical decode position from the physical KV-cell count. IMC therefore keeps
the logical plan, logical position, and physical snapshot accounting consistent
without pretending that media is an ordinary text token.

#### Step 3: Find the longest complete safe prefix

Kronk searches available sessions for the longest complete saved plan that is
a prefix of the new stable plan:

```text
New stable plan: [A B C D E F]

Session 1: [A B]          -> safe prefix
Session 2: [A B C D]      -> safe prefix and better
Session 3: [A B X]        -> not safe
Session 4: [A B C D E F]  -> exact and best
```

Choosing the longest compatible session minimizes new prefill work. A prefix is
safe only when:

- It ends at a complete, committed session boundary.
- The saved plan and snapshot metadata describe the same model state.
- The complete saved plan prefixes the new stable plan.
- The session is available rather than reserved by another request.
- For media, the saved media plan is unchanged and anything added after the
  anchor is text-only.

"Longest complete prefix" does not mean the longest coincidental token overlap.
For example, `[A B C D]` is not reused for `[A B X D]`, even though `[A B]`
matches. Kronk does not trim an existing session at an arbitrary internal point.
This conservative rule avoids assuming that an internal KV cut remains valid
across template boundaries, media embeddings, M-RoPE positions, hybrid
recurrent state, or draft/MTP state.

#### Step 4: Select exact, append, anchor, or rebuild

The selected action follows from the stable plan and the best safe session.

**Exact** means the cached stable plan and new stable plan are identical:

```text
Cached stable: [A B C D]
New stable:    [A B C D]
Actual:        [A B C D][G]
```

Kronk restores the snapshot and processes only `[G]`. Because the stable state
did not change, it can usually skip serializing the same snapshot again.

**Append** applies to a text-only stable extension:

```text
Cached stable: [A B C D]
New stable:    [A B C D E F]
Actual:        [A B C D E F][G]
```

Kronk restores `[A B C D]`, prefills `[E F]`, snapshots the new reusable state
`[A B C D E F]`, and then processes `[G]` without adding it to that stable
snapshot.

**Anchor** is the media-safe form of append:

```text
Cached plan: [A B][image-1][C D]
New plan:    [A B][image-1][C D E F]
```

The unchanged media-bearing plan acts as an anchor. Kronk restores its snapshot,
does not encode `image-1` again, prefills only `[E F]`, and stages an advanced
snapshot before processing the generation tail. The staged snapshot and its
matching metadata are published together only after success, so a failed
advance leaves the previous media snapshot authoritative.

Changing, appending, removing, or reordering media is not an anchor extension:

```text
[A B][image-2][C D]             -> rebuild: changed media
[A B][image-1][image-2][C D]    -> rebuild: appended media
[A B][C D][image-1]             -> rebuild: reordered media
[A B][C D]                      -> rebuild: removed media
```

**Rebuild** means no complete saved session prefixes the stable plan. Kronk
selects an empty session or the least recently used available session, resets
it, processes the stable plan from the beginning, snapshots the resulting
state, and then processes the generation tail. Rebuild is not a request
failure; it means only that the request receives no saved-prefill benefit.

#### Step 5: Reserve the selected session

Planning and execution do not happen at the same instant. A request may wait
for an execution slot after selecting a session, so Kronk immediately marks the
session reserved while holding the cache lock:

```text
select session -> reserve -> restore/extend/snapshot/generate -> release
```

Other planners skip reserved sessions. Reservations apply to exact matches,
appends, media anchors, empty sessions selected for rebuild, and LRU sessions
selected for replacement. Even an exact match needs a reservation: another
request must not rebuild or replace its snapshot between selection and restore.

If every session is reserved, Kronk returns a retryable busy error rather than
evicting an active session. The reservation also prevents a partially updated
session from becoming visible. Snapshot bytes and metadata must describe the
same stable prefix before the session can be selected again.

#### Why this is called prompt planning

The old mental model, "which messages have been cached?", is no longer precise
enough. The planner now produces an execution contract containing:

- The complete rendered prompt and reusable stable rendering
- The generation-only tail
- The canonical token or media identity
- The longest reusable session boundary
- The stable extension that must be prefilled
- The exact, append, anchor, or rebuild action
- Logical positions and expected physical KV state
- The selected session and the state version execution must observe

The batch engine executes this plan rather than independently interpreting a
reduced message document. Put another way:

> Prompt planning asks, "What exact prompt will the model execute, and what is
> the safest minimum work needed to reach it?" IMC supplies the reusable model
> state that makes the answer efficient.

## 5.2 How Kronk Reuses a Text Prefix

Kronk compares the complete stable token sequence with sequences retained by
existing sessions. The result is one of three match types:

- **Exact** — The new stable sequence is identical to a cached sequence. Kronk
  restores that session and processes only the generation-ready tail.
- **Append** — A cached sequence is a complete prefix of the new stable
  sequence. Kronk restores it, processes the appended stable tokens, and then
  processes the generation-ready tail.
- **Rebuild** — No complete cached sequence prefixes the new stable sequence.
  Kronk uses an empty session or replaces the least recently used available
  session and processes the stable prefix from the beginning.

Only complete-prefix reuse is allowed. If an earlier message is edited,
removed, reordered, or rendered differently, Kronk rebuilds the prefix. It does
not trim an existing session at an internal point and attempt to salvage the
tokens before the divergence.

For example:

```text
Cached stable tokens: [A B C D]

New stable tokens:    [A B C D]       -> exact
New stable tokens:    [A B C D E F]   -> append E F
New stable tokens:    [A B X D]       -> rebuild
```

This comparison uses rendered tokens, not only the message objects supplied by
the client. Changes to the chat template, tool definitions, thinking options,
or other inputs that affect rendering can therefore prevent reuse even when
the visible message text appears unchanged.

## 5.3 Sessions, Slots, and Snapshots

An IMC **session** is a reusable conversation identity and its saved model
state. An execution **slot** is a lane that can actively run a request. These
are deliberately separate:

- Kronk retains `max(nseq-max, 1) × max(3, queue-depth)` IMC sessions.
- Only `nseq-max` requests can decode concurrently.
- A session can be restored into any available execution slot; it is not tied
  permanently to one slot.
- Session storage is allocated lazily as conversations begin using it.

For example, `nseq-max: 2` with the default `queue-depth: 2` provides two
concurrent decode slots and six warm IMC session identities. A queue depth
greater than 3 expands the session pool so it remains at least as large as the
generation admission capacity. Raising `nseq-max` also increases the unified
KV cache capacity and its memory cost, so do not raise it solely to retain
more conversation branches without considering the effects described in
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing).

Admission waiting is controlled separately by the per-model
`admission-timeout` setting (default `3m`). It only bounds the wait for an SDK
admission permit. The server's `KRONK_WEB_INFERENCE_TIMEOUT` (default `60m`)
instead bounds admitted preparation, slot waiting, and inference; neither
setting changes IMC session retention.

Kronk reserves a session as soon as it selects it for an exact match, append,
or rebuild. Other requests cannot select that identity while the reservation
is held. If all session identities are reserved, the request returns a busy
error and should be retried. Kronk does not evict an active session to make
room.

During a request, Kronk restores the selected snapshot into a free slot. For a
new or appended stable prefix, it creates the updated snapshot after processing
the stable tokens. The generation-ready tail is then processed without making
it part of that reusable stable prefix.

An exact match may skip rewriting the snapshot when the stable state has not
changed. This avoids an unnecessary serialization of the state that was just
restored. Exact media-plan reuse can receive the same optimization. These are
implementation optimizations; they do not change which content is considered
part of the cache.

Snapshots externalize inactive session state from the model's active KV cache.
They therefore do not permanently occupy an execution slot or pin their state
in accelerator KV memory between requests. They do consume host or disk
storage, as described in [Configuration and Storage](#55-configuration-and-storage).

## 5.4 Media Requests

IMC supports media processed by Kronk's multimodal pipeline. Instead of relying
only on text-token equality, Kronk builds a logical plan containing the ordered
text and media inputs.

Kronk can reuse a media session in two cases:

- **Exact plan** — The complete stable media plan is unchanged.
- **Text extension from an anchor** — The stored media plan remains unchanged
  and is followed only by new text. Kronk restores the media state and processes
  the text extension without encoding the media again.

Kronk rebuilds the stable plan when media is changed, reordered, removed, or
newly appended. This conservative rule lets the model-specific multimodal
pipeline remain authoritative for media embeddings, token placement, and
position handling.

For example, a user can submit an image and then ask several text-only
follow-up questions. The saved media plan acts as an anchor for those turns.
Replacing the image or adding another one requires a rebuild.

See [Chapter 11](https://www.kronkai.com/manual#chapter-11-multimodal-models) for supported media inputs and
model requirements.

## 5.5 Configuration and Storage

IMC settings belong under the model ID in
`~/.kronk/models/model_config.yaml`:

```yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  cache-min-tokens: 100
  session-store-kind: ram
```

The relevant settings are:

| Setting              | Default | Description                                                         |
| -------------------- | ------- | ------------------------------------------------------------------- |
| `incremental-cache`  | `true`  | Enables IMC for the model.                                          |
| `cache-min-tokens`   | `100`   | Minimum stable-render length required to create or reuse a session. |
| `session-store-kind` | `ram`   | Stores inactive session snapshots in `ram` or on `disk`.            |
| `session-store-dir`  | None    | Existing writable directory required by the `disk` store.           |

The `cache-min-tokens` setting applies to the stable-render token length. A
request below the threshold still works, but Kronk processes its complete
generation-ready prompt without creating or reusing an IMC session.

Set `incremental-cache: false` if a workload is entirely short-lived or if you
need to compare behavior without prompt caching.

### RAM storage

The default `ram` store keeps snapshots in process memory. Each session buffer
grows as needed and retains its peak allocation for reuse. Actual memory use
depends on the model, cached conversation lengths, KV data types, and number of
sessions that have been used. Budget for peak conversation state across the
branches you expect to keep warm, not just the `nseq-max` requests that can run
simultaneously.

### Disk storage

To place inactive snapshots on disk:

```yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  session-store-kind: disk
  session-store-dir: /var/lib/kronk/sessions
```

The directory must already exist and be writable by the Kronk process. Kronk
creates a temporary file for each used session and removes it during a normal
model unload. Files can remain after a process crash, so use a dedicated
directory and arrange cleanup appropriate for your deployment.

Disk storage changes where inactive snapshots are retained, but it does not
eliminate snapshot-sized RAM usage. Snapshot and restore operations require
memory buffers, and a session can retain buffers sized to its largest state.
Disk also adds I/O latency. Measure both memory and request latency with your
model and storage device before relying on it as a capacity solution.

Some MTP configurations maintain draft-model cached state and saved hidden
state in addition to the target model snapshot. Account for this extra storage
when sizing memory. See [Chapter 6](https://www.kronkai.com/manual#chapter-6-speculative-decoding-and-mtp) for
MTP configuration and behavior.

## 5.6 Invalidation and Limitations

IMC favors safe reuse over partial recovery. A session is rebuilt when Kronk
cannot prove that its complete saved prefix matches the new stable prompt.
Common causes include:

- Editing, deleting, or reordering earlier conversation content
- Changing tools or settings that alter the rendered prompt
- Changing, adding, removing, or reordering media
- Loading a different model or an incompatible model configuration
- Producing a stable rendering that is not a prefix of the generation-ready
  rendering

An unload or server restart clears in-memory sessions. The disk store is an
inactive snapshot backend, not a persistent conversation database; do not rely
on IMC sessions surviving model or process lifecycles.

IMC has several practical costs:

- Planning text reuse requires rendering and tokenizing complete prompts.
- Snapshot and restore operations use host memory bandwidth and, for disk
  storage, filesystem I/O.
- Edited text rebuilds instead of reusing an arbitrary partial prefix.
- The session pool is finite, so inactive least-recently-used branches can be
  replaced as new branches arrive.
- MTP can require additional draft-side state. If Kronk restores the target
  prefix without compatible draft state, it can still use the target cache but
  disables speculative decoding for that request.

Evaluate IMC using a representative conversation workload rather than a single
prompt benchmark. The benefit grows with reusable prefix length and follow-up
frequency.

## 5.7 Observability

At debug log level, IMC planning events identify the selected `match_kind`
(`exact`, `append`, or `rebuild`) and report reusable, extension, stable, and
tail token counts. Media planning events similarly identify exact, anchor, and
rebuild decisions. Request-completion events include whether IMC participated
and whether a prior snapshot was restored.

The Prometheus counters `imc_snapshot_skipped_total` and
`imc_pure_hit_stale_session_total` expose exact-hit snapshot skips and rejected
stale-session races. A rising rebuild rate usually means clients are changing
earlier prompt content, media, tools, or rendering inputs rather than appending
to a stable conversation.

See [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability) for logging, metrics, tracing, and
profiling configuration.
