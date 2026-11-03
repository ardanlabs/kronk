# Lifecycle and Inference Invariants

Part of `kronk-service-architecture`. Chapter 20 is the only full statement of these rules; this file tells you which section to read. When the diff touches an area below, read its section before writing or reviewing, then check the change against every rule in it. The last column is only the rule most often broken, not a summary.

General rule: the layer that creates a goroutine, stream, native object, admission permit, active-use count, or reservation releases it on every return path, including error and cancellation.

| Area | Applies when the diff touches | Read | Most often broken |
|------|-------------------------------|------|-------------------|
| Admission and active use | `sdk/kronk` handle methods, streaming, unload | [§20.5.3](../../../../.manual/chapter-20-developer-guide.md#2053-semaphore-lifetime-and-cancellation) | Permit released at stream construction, or leaked on an early error |
| Pools and reservations | `sdk/pool`, `sdk/pool/engine`, `sdk/*/pool`, service pool wiring | [§20.5.2](../../../../.manual/chapter-20-developer-guide.md#2052-typed-pool-acquisition-loading-and-eviction), [§20.3.3](../../../../.manual/chapter-20-developer-guide.md#2033-pools-and-shared-resources) | Reservation kept after a failed load, or a partial entry published |
| Batch slots and sequences | `sdk/kronk/model` batch engine files | [§20.5.4](../../../../.manual/chapter-20-developer-guide.md#2054-batch-slots-and-sequence-isolation) | Parser, sampler, media, or speculative state leaking into the next job on the slot |
| Embedding and rerank engine | sequence-batch engine, `supportsBatchSeq`, `embedapp`/`rerankapp` | [§20.5.5](../../../../.manual/chapter-20-developer-guide.md#2055-generation-versus-embedding-and-reranking-engines) | Runtime fallback heuristics added in a handler |
| IMC and `SessionStore` | `caching_imc*`, `kvstorage`, session/system cache code | [§20.6.1](../../../../.manual/chapter-20-developer-guide.md#2061-imc-sessions-slots-and-external-storage) | Session metadata updated without its snapshot, or a borrowed `Bytes` result retained |
| Prompt plans and media | prompt planning, `media*`, mtmd paths | [§20.6.2](../../../../.manual/chapter-20-developer-guide.md#2062-prompt-plans-text-and-media) | A media prompt reused on a text-prefix match |
| Parsers | `sdk/kronk/parsers/**`, parser registry in `model` | [§20.6.3](../../../../.manual/chapter-20-developer-guide.md#2063-parser-registry-ownership) | Untested input split across stream chunks |
| Responses normalization | `sdk/kronk/response.go` | [§20.6.4](../../../../.manual/chapter-20-developer-guide.md#2064-responses-normalization) | "Cleaning up" the in-place `model.D` mutation into a copy or rebuild |
| Speculative decoding and MTP | `draft*`, `model/internal/speculation/**` | [§20.6.6](../../../../.manual/chapter-20-developer-guide.md#2066-speculative-decoding-and-mtp) | Mutating one slot before every slot's verification rows are read |
| Tracing, logging, metrics | `observ/**`, spans, log lines, `mid` | [§20.6.5](../../../../.manual/chapter-20-developer-guide.md#2065-tracing-and-logging) | Span per token, or user content logged without insecure logging |
| Bucky | `sdk/bucky/**`, `audioapp` | [§20.8.2](../../../../.manual/chapter-20-developer-guide.md#2082-lifecycle-invariants) | Model state or capacity not released on a failed transcription or stream |
| Routes and middleware | `route.go`, `mid`, `mux` | [§20.7.1](../../../../.manual/chapter-20-developer-guide.md#2071-routes-middleware-and-domains) | A route registered without its access middleware |

For verification of each area, use [§20.9.2](../../../../.manual/chapter-20-developer-guide.md#2092-choosing-effective-checks).
