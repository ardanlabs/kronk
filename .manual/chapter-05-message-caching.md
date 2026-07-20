# Chapter 5: Message Caching

## Table of Contents

- [5.1 What IMC Does](#51-what-imc-does)
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

For text requests, Kronk creates two complete prompt renderings:

1. A **stable rendering** without the generation prompt. This is the reusable
   prefix stored by IMC.
2. A **generation-ready rendering** used for inference. This includes a
   nonempty tail after the stable prefix.

The stable tokens must be a prefix of the generation-ready tokens. This lets
Kronk reuse a complete, template-valid conversation rather than rendering an
independent suffix that might have different template semantics.

The `cache-min-tokens` setting controls the minimum stable-render token length
required to create or reuse an IMC session. Its default is 100. Requests below
the threshold still work, but Kronk processes the complete generation-ready
prompt without IMC.

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

- Kronk retains up to `nseq-max × 3` IMC sessions.
- Only `nseq-max` requests can decode concurrently.
- A session can be restored into any available execution slot; it is not tied
  permanently to one slot.
- Session storage is allocated lazily as conversations begin using it.

For example, `nseq-max: 2` provides two concurrent decode slots and up to six
warm IMC session identities. Raising `nseq-max` also increases the unified KV
cache capacity and its memory cost, so do not raise it solely to retain more
conversation branches without considering the effects described in
[Chapter 4](chapter-04-batch-processing.md).

Kronk reserves a session as soon as it selects it for an exact match, append,
or rebuild. Other requests cannot select that identity while the reservation
is pending. If all session identities are pending, the request returns a busy
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

See [Chapter 11](chapter-11-multi-modal-models.md) for supported media inputs and
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

| Setting | Default | Description |
| ------- | ------- | ----------- |
| `incremental-cache` | `true` | Enables IMC for the model. |
| `cache-min-tokens` | `100` | Minimum stable-render length required to create or reuse a session. |
| `session-store-kind` | `ram` | Stores inactive session snapshots in `ram` or on `disk`. |
| `session-store-dir` | None | Existing writable directory required by the `disk` store. |

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
when sizing memory. See [Chapter 6](chapter-06-speculative-decoding-mtp.md) for
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

See [Chapter 15](chapter-15-observability.md) for logging, metrics, tracing, and
profiling configuration.
