# Chapter 5: Message Caching

## Table of Contents

- [5.1 What IMC Does](#51-what-imc-does)
  - [5.1.1 Quick Semantic Understanding](#511-quick-semantic-understanding)
- [5.2 How Kronk Reuses a Text Prefix](#52-how-kronk-reuses-a-text-prefix)
  - [5.2.1 User-Boundary Lifecycle and Template Behavior](#521-user-boundary-lifecycle-and-template-behavior)
- [5.3 Sessions, Slots, and Snapshots](#53-sessions-slots-and-snapshots)
- [5.4 Media Requests](#54-media-requests)
- [5.5 Configuration and Storage](#55-configuration-and-storage)
- [5.6 Invalidation and Limitations](#56-invalidation-and-limitations)
- [5.7 Observability](#57-observability)

---

## 5.1 What IMC Does

Chapter 4 explains where IMC participates in the generation lifecycle. This
chapter explains how Kronk chooses reusable state and preserves it safely.

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

IMC caches the tokens used for prefix matching and the corresponding serialized
model sequence state—principally KV-cache tensors, plus recurrent or MTP state
where required. The distinction matters: token equality proves which saved
prefix is eligible, while the serialized state is what avoids processing that
prefix again.

```text
Current snapshot
├─ prefix-matching token IDs
└─ serialized model sequence state
   ├─ KV-cache tensors
   ├─ recurrent/SSM state for hybrid models
   ├─ logical positions
   ├─ draft/MTP state when compatible
   └─ model-specific snapshot encoding
```

Not every model needs every listed component. Kronk stores the state required
to resume the configured target and, when compatible, its draft path. Prompt
metadata is retained alongside the snapshot so Kronk can prove that the bytes
belong to the same complete rendered prefix.

This is prompt-oriented rather than message-oriented because the model does not
consume message objects directly. It consumes rendered tokens, media
embeddings, and positions. Requests with apparently unchanged messages can
render differently when tools, thinking settings, templates, media, or other
render-affecting inputs change.

Each text session can own two complete snapshots:

- **Current** is the latest complete stable prompt state and is the normal
  append path.
- **User Boundary** is one additional verified state retained as a recovery
  point. Metrics and the BUI call this state **Fallback** because it is selected
  when Current cannot be reused.

The fallback exists because adding a new real user message can cause a chat
template to re-render earlier assistant messages differently. A reasoning
template may preserve thinking for the latest assistant message but omit it
once that message becomes historical. The message history grew, yet its newly
rendered token sequence can be shorter or can diverge before the new user
message. Current is then unusable even though the conversation was only
appended.

Kronk never restores an arbitrary longest common prefix (LCP). It restores only
a complete Current or User Boundary snapshot whose full token sequence prefixes
the new stable rendering. If neither qualifies, Kronk rebuilds from the
beginning. The LCP can prove that a proposed user-turn boundary is unchanged,
but the LCP itself is not model state and is not a safe restoration boundary.

The complete generation lifecycle is introduced in
[Chapter 4 §4.3](https://www.kronkai.com/manual#43-the-generation-inference-lifecycle).
The focused view below zooms into IMC's Stage 2 responsibility: deciding the
safest minimum work and reserving the session that owns reusable state.

![IMC session selection using complete saved prompt plans](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-05/imc-session-selection.png)

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

After tokenization or media-plan construction, the stable plan must be a strict
prefix of the generation-ready plan, and the generation-ready plan must
contribute a nonempty tail. For media, that tail must also be text-only. Kronk
renders both complete prompts instead of independently rendering a suffix
because a chat template can make separators, control tokens, and role
formatting depend on the surrounding messages. If the resulting plans are not
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
a prefix of the new stable plan. A text session can retain its Current snapshot
and one User Boundary snapshot. This is one current state plus one fallback,
not an arbitrary collection of token checkpoints:

```text
New stable plan: [A B C D E F]

Session 1 Current:  [A B]          -> safe prefix
Session 2 Current:  [A B C D]      -> safe prefix and better
Session 3 Current:  [A B X]        -> not safe
Session 4 Current:  [A B C D E F]  -> exact and best
Session 3 Boundary: [A B C]        -> also safe
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
For example, `[A B C D]` is not shortened and reused for `[A B X D]`, even
though `[A B]` matches. The LCP is not selected as the boundary. Kronk restores
only model state that it explicitly serialized at a complete Current or User
Boundary. This avoids assuming that an internal KV cut remains valid across
template boundaries, media embeddings, M-RoPE positions, hybrid recurrent
state, or draft/MTP state.

When the request ends with a new real user message, Kronk also independently
renders and tokenizes the history with that final message removed. This tests
the boundary immediately before the new user message:

```text
[SYSTEM][USER1][ASSISTANT][USER2]
                         ^
                         candidate boundary
```

The candidate is accepted only when those independently rendered tokens are an
exact prefix of the complete stable request and remain equal through the same
position in an available saved Current sequence. If extending from an earlier
saved state, Kronk can prefill to that verified boundary, serialize fresh
target and compatible draft/MTP state there, and continue to the complete
Current state. It never extracts a checkpoint merely because an LCP ended at
that token. This progressive boundary may end after an assistant message; the
name means “the verified transition immediately before the real user message,”
not “a snapshot whose final role must be user.”

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

If Current diverges because a template retroactively changes historical
rendering, append can instead begin from User Boundary. Kronk restores that
complete state and prefills everything after it; it never rewinds a later model
state or deletes an arbitrary KV range.

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

These outcomes describe comparison with the shown anchor. Because Kronk
searches the full available session pool, it can still reuse another session
whose complete media plan is compatible with the request.

**Rebuild** means neither Current nor User Boundary prefixes the stable plan.
Kronk selects an empty session or the least recently used available session,
resets it, processes the stable plan from the beginning, snapshots the
resulting state, and then processes the generation tail. Rebuild is not a
request failure; it means only that the request receives no saved-prefill
benefit.

#### Step 5: Reserve the selected session

Planning and execution do not happen at the same instant. A request may wait
for an execution slot after selecting a session, so Kronk immediately marks the
session reserved while holding the cache lock:

```text
select session -> reserve -> queue -> restore/build stable state

ordinary text append/rebuild or initial media build:
    publish complete stable snapshot -> release -> generate

exact read-only hit or media-anchor advance:
    keep reservation -> generate -> release at completion
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

Kronk compares the complete stable token sequence with the Current and User
Boundary sequences in available sessions. The result is one of three match
types:

- **Exact** — The new stable sequence is identical to a cached sequence. Kronk
  restores that session and processes only the generation-ready tail.
- **Append** — A cached sequence is a complete prefix of the new stable
  sequence. Kronk restores it, processes the appended stable tokens, and then
  processes the generation-ready tail.
- **Rebuild** — No complete cached sequence prefixes the new stable sequence.
  Kronk uses an empty session or replaces the least recently used available
  session and processes the stable prefix from the beginning.

Only complete-prefix reuse is allowed. If an earlier message is edited,
removed, reordered, or rendered differently, Kronk can reuse a retained User
Boundary only when that entire state still prefixes the new rendering. Kronk
does not turn a coincidental partial token match into cached model state.

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

### 5.2.1 User-Boundary Lifecycle and Template Behavior

A **real user message** is a user-authored turn, not a tool response represented
with the `user` role. Kronk uses real user messages because they begin a new
model thought and provide a meaningful recovery boundary around assistant and
tool activity.

The normal lifecycle is:

1. A first request such as `[SYSTEM][USER1]` is built and published as Current.
   No User Boundary is required yet.
2. When that exact Current prefixes a later request, Kronk restores and extends
   it. If Current ended at a real user message, Kronk preserves that complete
   state as User Boundary before publishing the longer Current.
3. If a later rendering still starts with Current, Kronk continues extending
   Current and can advance User Boundary to the most recent verified state.
4. If Current no longer prefixes the rendering but User Boundary does, Kronk
   restores User Boundary, prefills the extension, retains the verified
   boundary, and publishes the rebuilt Current.
5. If neither snapshot is an exact prefix, Kronk rebuilds from the beginning.

For example:

```text
After request 1:
  Current:       [SYSTEM][USER1]
  User Boundary: none

After compatible assistant/tool activity and USER2:
  Current:       [SYSTEM][USER1][ASSISTANT/TOOLS][USER2]
  User Boundary: [SYSTEM][USER1]

After the next compatible extension:
  Current:       [SYSTEM][USER1][ASSISTANT/TOOLS][USER2][ASSISTANT/TOOLS]
  User Boundary: [SYSTEM][USER1][ASSISTANT/TOOLS][USER2]
```

The exact token lengths depend on the template; the brackets show logical
message boundaries, not token counts. With a deterministic, prefix-stable
template, each Current prefixes the next full rendering, so Kronk restores it,
appends only the new stable tokens, and advances the retained boundary.

![IMC cache growth with a deterministic template](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-05/imc-cache-growth-deterministic.png)

A Qwen-like reasoning template can behave differently at the candidate
boundary immediately before a new user message. When Kronk independently
renders the history without that user message, the preceding assistant is the
last message and its `reasoning_content` is emitted. In the full rendering the
new user follows that assistant, so the same reasoning may be omitted or moved.
The boundary-only rendering therefore does not prefix the full stable rendering.

Kronk rejects that candidate boundary. It does not trim model state at the
coincidental common token prefix. It restores only an earlier serialized
complete state that still prefixes the full rendering, recomputes forward, and
publishes the new Current state. A completed Current that ended at a real user
message can be preserved as the next User Boundary on a later extension.

![IMC cache growth with a Qwen-like reasoning template](https://raw.githubusercontent.com/ardanlabs/kronk/main/.manual/images/chapter-05/imc-cache-growth-qwen.png)

Kronk may also build a newer User Boundary while prefilling a request that ends
in a real user message. It independently renders the history without that final
user message and verifies that the resulting tokens are an exact prefix of the
full stable rendering and the previously saved tokens. Only then does it
serialize model state at that boundary. This verification is important for
templates whose output depends on which assistant message is last: the
independent boundary render is useful only when it produces the same bytes in
the full request.

Internally, selecting User Boundary temporarily swaps snapshot ownership so the
normal Current restore path can execute. This bookkeeping does not mean the
old divergent Current is a valid fallback. While a request is rebuilding, the
BUI reports the fallback kind as `calculating`; after a verified boundary is
published it reports `user`. The Fallback Updates value counts installations
and advances for that cache entry and resets when the bounded entry is
recycled.

## 5.3 Sessions, Slots, and Snapshots

An IMC **session** is a reusable conversation identity and its saved model
state. An execution **slot** is a lane that can actively run a request. These
are deliberately separate:

- By default, Kronk retains `max(nseq-max, 1) × max(3, queue-depth)` IMC
  sessions; `imc-session-capacity` can override that count.
- Only `nseq-max` requests can decode concurrently.
- A session can be restored into any available execution slot; it is not tied
  permanently to one slot.
- Snapshot bytes and backing storage are allocated lazily as conversations
  begin using them.

For example, `nseq-max: 2` with the default `queue-depth: 2` provides two
concurrent decode slots and six warm IMC session identities. A queue depth
greater than 3 expands the session pool so it remains at least as large as the
generation admission capacity. Raising `nseq-max` also adds another full
`context-window` KV stream and its memory cost, so do not raise it solely to
retain more conversation branches without considering the effects described in
[Chapter 4](https://www.kronkai.com/manual#chapter-4-batch-processing).

An explicit `imc-session-capacity` must be at least
`nseq-max × queue-depth`. This preserves one reservable session identity for
every admitted generation request. Values above that floor retain more
completed conversation branches and may reduce LRU rebuilds, at the cost of
additional session-store capacity.

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

The snapshot restores model state, not a long-lived sampler instance. Kronk
re-primes repetition penalties and DRY from the authoritative complete logical
prompt, including the restored portion, so sampling history agrees with the KV
prefix even though only uncached model work is decoded again. For cached media,
Kronk retains the text-token history needed for that sampler priming separately
from the media embedding cells represented by the native snapshot.

For text sessions, Kronk can retain one User Boundary (reported as Fallback) in
addition to Current. It includes matching draft/MTP state when available. Kronk
may create it by preserving a Current state that ended at a real user message,
or by prefilling and serializing an independently verified boundary immediately
before the final real user message. Publishing a new User Boundary replaces the
prior one. It can require approximately one additional snapshot-sized
allocation for each active logical session, making the maximum two complete
states: Current plus User Boundary.

An exact match may skip rewriting the snapshot when the stable state has not
changed. This avoids an unnecessary serialization of the state that was just
restored. Exact media-plan reuse can receive the same optimization. These are
implementation optimizations; they do not change which content is considered
part of the cache.

Snapshots externalize inactive session state from the model's active KV cache.
They therefore do not permanently occupy an execution slot or pin their state
in accelerator KV memory between requests. They do consume the configured
session-store capacity, as described in
[Configuration and Storage](#55-configuration-and-storage).

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
newly appended and no other available session already contains a compatible
plan. This conservative rule lets the model-specific multimodal pipeline remain
authoritative for media embeddings, token placement, and position handling.

For example, a user can submit an image and then ask several text-only
follow-up questions. The saved media plan acts as an anchor for those turns.
Replacing the image or adding another one cannot extend that anchor; it requires
a rebuild unless a different session already holds the new plan.

See [Chapter 11](https://www.kronkai.com/manual#chapter-11-multimodal-models) for supported media inputs and
model requirements.

## 5.5 Configuration and Storage

IMC settings belong under the model ID in
`~/.kronk/models/model_config.yaml`:

```yaml
Qwen/Qwen3-8B-Q8_0:
  incremental-cache: true
  cache-min-tokens: 100
  imc-session-capacity: 8
  session-store-kind: ram
```

The relevant settings are:

| Setting              | Default | Description                                                         |
| -------------------- | ------- | ------------------------------------------------------------------- |
| `incremental-cache`  | `true`  | Enables IMC for the model.                                          |
| `cache-min-tokens`   | `100`   | Minimum stable text-token-plan length required for text IMC reuse.  |
| `imc-session-capacity` | Derived | Reusable identities; must be at least admission capacity.          |
| `session-store-kind` | `ram`   | Selects the session-store plugin. Currently, only `ram` is built in. |

When `imc-session-capacity` is omitted, Kronk derives the capacity from
`nseq-max` and `queue-depth`. The same admission-capacity floor applies to an
explicit value in `model_config.yaml`.

The `cache-min-tokens` setting applies to the stable text-token-plan length. A
text request below the threshold still works, but Kronk processes its complete
generation-ready prompt without creating or reusing an IMC session. The current
media planner does not apply this threshold; media safety is instead determined
by whether it can construct compatible logical stable and actual plans.

Set `incremental-cache: false` if a workload is entirely short-lived or if you
need to compare behavior without prompt caching.

### Built-in RAM storage

The built-in `ram` store keeps snapshots in process memory. It is selected by
default when `session-store-kind` is omitted. Each session buffer
grows as needed and retains its peak allocation for reuse. Actual memory use
depends on the model, cached conversation lengths, KV data types, and number of
sessions that have been used. Budget for peak conversation state across the
branches you expect to keep warm, not just the `nseq-max` requests that can run
simultaneously.

### Custom SDK storage

Direct SDK users can implement `kvstorage.Store`, construct a factory that
captures the implementation's own dependencies and configuration, and inject
that factory into the model:

```go
factory := func() (kvstorage.Store, error) {
	return myStore.New(dependency)
}

krn, err := kronk.New(
	model.WithModelFiles(modelFiles),
	model.WithIncrementalCache(true),
	model.WithSessionStoreFactory(factory),
)
```

Kronk calls the factory independently for every Current, draft, and Fallback
store it needs. Each call must return a new store; Kronk owns that store and
calls `Close` when it is no longer needed. Direct SDK use defaults to RAM when
no factory is injected.

The [`examples/session-store`](https://github.com/ardanlabs/kronk/tree/main/examples/session-store)
program provides a complete custom implementation and shows how to inject it.
Its implementation writes snapshots to anonymous temporary files and deletes
them on `Close`. It exists only to demonstrate the extension contract: it has
no stable session identity, persisted request history, startup recovery,
atomic commits, or reliable way to report I/O failures. **Do not use the
example as durable session storage.** A durable implementation needs a higher
level persistence design in addition to the byte-store contract.

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
- For media, failing to construct an append-safe logical plan, including a
  marker/media-count mismatch, an automatically appended EOS, or a non-text
  generation tail

An unload or server restart clears the built-in in-memory sessions. Do not rely
on IMC sessions surviving model or process lifecycles.

IMC has several practical costs:

- Planning text reuse requires rendering and tokenizing complete prompts.
- Snapshot and restore operations use host memory bandwidth.
- Edited text rebuilds instead of reusing an arbitrary partial prefix.
- The session pool is finite, so inactive least-recently-used branches can be
  replaced as new branches arrive.
- MTP can require additional draft-side state. If Kronk restores the target
  prefix without compatible draft state, it can still use the target cache but
  disables speculative decoding for that request.
- A corrupt, empty, or partial target snapshot is never treated as a shorter
  reusable prefix. Kronk invalidates the affected current state and fails that
  restore so a later request can rebuild safely. An independently retained
  Fallback text snapshot remains available; a failed staged media-anchor
  advance leaves its previously published media snapshot authoritative.

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

Per-entry gauges expose Current and Fallback token/allocation values, Fallback
kind and update count, latest-request input/output/context, peak context, and
context-window utilization. `imc_session_fallback_kind` uses `user` for a
verified real-user boundary and `calculating` for transient snapshot ownership
while Current is being rebuilt. The series is absent when no Fallback exists.
These are bounded current-state gauges, not a history of every conversation
transition.

See [Chapter 15](https://www.kronkai.com/manual#chapter-15-observability) for logging, metrics, tracing, and
profiling configuration.
