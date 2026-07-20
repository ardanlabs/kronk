# Chapter 6: Speculative Decoding and MTP

## Table of Contents

- [6.1 What Speculative Decoding Does](#61-what-speculative-decoding-does)
- [6.2 Drafter Sources and Selection](#62-drafter-sources-and-selection)
- [6.3 Choosing a Drafter](#63-choosing-a-drafter)
- [6.4 Draft Size and Adaptive Throttling](#64-draft-size-and-adaptive-throttling)
- [6.5 Configuration](#65-configuration)
- [6.6 Measuring the Result](#66-measuring-the-result)
- [6.7 Limitations and Fallbacks](#67-limitations-and-fallbacks)

---

### 6.1 What Speculative Decoding Does

Speculative decoding uses a faster drafter to propose several continuation
tokens. The target model verifies those proposals together. Accepted proposals
reduce the number of target-model passes needed to produce the response;
rejected proposals are discarded and the target remains authoritative.

This optimization does not change the chat, Responses, or SDK request shapes.
It can improve generation throughput when the drafter is inexpensive and its
proposals agree frequently with the target. It can also reduce performance when
draft work is expensive or acceptance is poor. Always measure with the model,
sampling settings, hardware, and prompts used in production.

Kronk supports classic speculative decoding with a separate draft model and
Multi-Token Prediction (MTP). MTP uses a prediction head designed for the
target rather than a general-purpose smaller language model.

### 6.2 Drafter Sources and Selection

Kronk can load a drafter from three sources:

| Source | How it is supplied | Slots |
| ------ | ------------------ | ----- |
| **Classic separate draft** | A `draft-model` configuration names another compatible GGUF. | Requires `nseq-max: 1` |
| **Companion MTP assistant** | A model-specific assistant GGUF, currently used by Gemma4 models, is discovered with the downloaded target. | Supports multiple slots |
| **Embedded MTP head** | The target GGUF contains supported `nextn_predict_layers` metadata, currently used by Qwen3.5/Qwen3.6 models. | Supports multiple slots |

Kronk checks these sources in that order. A `draft-model` block containing a
`model-id` explicitly selects the classic separate draft and takes precedence
over either MTP form. Without one, Kronk uses a compatible companion MTP file
when present, then checks the target for an embedded MTP head. If no source is
available, the model runs normally without speculation.

A `draft-model` block containing only `ndraft` is different: it changes the MTP
draft ceiling and does not select a classic draft or disable MTP.

MTP also requires support from the loaded llama.cpp library. When a model
advertises MTP but the required API is unavailable, Kronk reports that MTP was
disabled at model load and serves the model without speculation.

### 6.3 Choosing a Drafter

#### 6.3.1 Classic separate draft

Use a classic separate draft when the target has no supported MTP source or
when you have already measured a compatible draft that performs well for your
workload.

The draft must use the same tokenizer and token IDs as the target. Kronk checks
that their vocabulary sizes match, but equal sizes alone cannot prove complete
tokenizer compatibility. Select models documented as compatible and test them
together. Similar names, parameter counts, or model families are not sufficient
evidence by themselves.

A separate draft has additional model and KV-cache memory costs. It also limits
the target entry to one execution slot. It is therefore a poor fit when:

- The target is already fast enough that drafting overhead dominates
- The workload needs `nseq-max` greater than 1
- No demonstrably tokenizer-compatible draft is available
- Measured throughput or latency is worse with drafting enabled

Do not select a draft using a universal model-pair or quantization rule.
Acceptance and cost can change substantially with sampling parameters and task
type.

#### 6.3.2 MTP

MTP is normally the simpler choice when the downloaded model provides a
supported embedded or companion head. It is architecture-matched to its target,
supports multiple execution slots, and does not require a `model-id` in the
`draft-model` configuration.

An embedded head requires no companion file. A companion MTP assistant is an
additional model-specific file, but Kronk's catalog and download flow can
discover and associate it with the target automatically. It is not configured
as a classic `draft-model`.

MTP availability is a property of the downloaded files and the loaded
llama.cpp library. Naming a model “MTP” or adding an `ndraft` override cannot
create an MTP head that is not present.

### 6.4 Draft Size and Adaptive Throttling

`ndraft` is the maximum number of candidates the drafter attempts in one
round. Larger values can save more target passes when acceptance remains high,
but they also increase wasted draft and verification work when proposals are
rejected.

Defaults are:

- **Classic separate draft:** 5
- **MTP:** 2

Kronk adapts the actual draft count independently for each execution slot. It
tracks an exponential moving average (EMA) of recent acceptance and chooses the
next round's size from the configured ceiling:

| Acceptance EMA | Next draft size |
| -------------- | --------------- |
| Below 0.30 | Usually 0 |
| 0.30 to below 0.50 | At most 1 |
| 0.50 to below 0.70 | At most 2 |
| 0.70 to below 0.85 | At most 3 |
| 0.85 or higher | Configured ceiling |

When the EMA is below 0.30, Kronk normally bypasses speculation. It performs a
one-token recovery probe every 32 fully throttled rounds so a slot can detect
that its workload has become predictable again.

The EMA belongs to the execution slot and persists across requests assigned to
that slot. A request can therefore begin with a reduced draft size after prior
requests on the same slot had poor acceptance. This is expected adaptive
behavior, not evidence that the configured ceiling was ignored.

### 6.5 Configuration

Configuration belongs under the target model ID in
`~/.kronk/models/model_config.yaml`.

#### 6.5.1 Classic separate draft

```yaml
some-provider/target-model:
  nseq-max: 1
  draft-model:
    model-id: some-provider/compatible-draft-model
    ndraft: 5
```

The target and draft must already be downloaded. Kronk resolves the configured
draft model ID to its local files. A classic draft with `nseq-max` greater than
1 is rejected during configuration validation.

#### 6.5.2 MTP default

No model configuration is required. Downloading a supported target and its
catalog-provided companion files is sufficient for automatic detection.

#### 6.5.3 MTP draft-count override

To change the MTP ceiling, set `ndraft` without a `model-id`:

```yaml
some-provider/mtp-target-model:
  draft-model:
    ndraft: 6
```

This form supports multiple slots. A value of 0 or an omitted value uses the
MTP default of 2; a negative value is rejected. If neither a compatible
companion nor an embedded MTP head is available, the override has no effect.
The adaptive throttle can still select fewer candidates than the configured
value.

See [Chapter 3](chapter-03-model-configuration.md) for the complete model
configuration format.

### 6.6 Measuring the Result

Do not use acceptance rate alone to decide whether speculation helps. Review
acceptance, coverage, throughput, latency, and resource use together.

The response `usage` object can include:

| Field | Meaning |
| ----- | ------- |
| `draft_tokens` | Candidate tokens proposed during the request |
| `draft_accepted_tokens` | Proposed tokens accepted by the target |
| `draft_acceptance_rate` | Accepted candidates divided by proposed candidates |
| `draft_coverage` | Fraction of output positions produced through speculative rounds |
| `draft_disable_reason` | Why MTP fell back to target-only execution during the request |

These fields use `omitempty`; zero-valued fields may be absent from JSON. A
high acceptance rate with low coverage can mean speculation ran for only a
small part of the response. Conversely, a moderate acceptance rate can still
help when drafting is much cheaper than target decoding. Compare end-to-end
latency or tokens per second against the same workload with no drafter.

Final `chat-completion` logs use `acceptance_rate`, while batch-engine completion
logs use `draft_acceptance_rate`. Both also report draft and accepted-token
counts when a drafter is loaded. Common MTP disable reasons include:

- `imc-hit` — target cache state was restored without compatible draft state
- `media-mrope` — MTP is not enabled for that M-RoPE media request
- `sync-error` — draft state could not be synchronized after verification

Startup events under `draft-model`, `draft-model-mtp`, and
`draft-model-mtp-shared` show which source loaded or why MTP was skipped. See
[Chapter 15](chapter-15-observability.md) for logging and metrics configuration.

### 6.7 Limitations and Fallbacks

- **Classic separate drafts require one slot.** Set `nseq-max: 1` on the
  target entry.
- **Tokenizer compatibility remains the user's responsibility.** Kronk rejects
  unequal vocabulary sizes, but that check cannot establish identical token
  mappings or templates.
- **MTP at nonzero temperature is an approximation.** MTP proposals are greedy,
  while target verification uses the request's sampler and accepts exact token
  matches. Sampling parameters still shape output, but this does not provide
  strict speculative-sampling distribution equivalence.
- **MTP can fall back per request.** A synchronization or compatible-state
  problem disables MTP for the affected request while target-only generation
  continues. It does not make an incorrect draft token authoritative.
- **IMC may restore target state without draft state.** Target-prefix reuse
  remains valid, but own-KV MTP runs target-only for that request when its draft
  snapshot is absent or cannot be restored. See
  [Chapter 5](chapter-05-message-caching.md).
- **Media support is conservative.** Media projection and media prefill run on
  the target. Unsupported own-KV media combinations and all M-RoPE media
  requests run without MTP, although target IMC can remain active. See
  [Chapter 11](chapter-11-multi-modal-models.md).
- **Drafting consumes resources.** A classic draft loads another model and KV
  cache. MTP heads and companion assistants also require compute and memory.
  Automatic detection does not guarantee a performance improvement.

Implementation details for drafting, verification, state synchronization, and
hybrid-model rollback belong in
[Chapter 19](chapter-19-developer-guide.md#1912-mtp-internals).
