# Chapter 7: YaRN Extended Context

## Table of Contents

- [7.1 Context Size and RoPE Scaling](#71-context-size-and-rope-scaling)
- [7.2 When to Use YaRN](#72-when-to-use-yarn)
- [7.3 Qwen3 Configuration](#73-qwen3-configuration)
- [7.4 Scaling Types and Parameters](#74-scaling-types-and-parameters)
- [7.5 Memory and Concurrency](#75-memory-and-concurrency)
- [7.6 Validate Quality](#76-validate-quality)

---

## 7.1 Context Size and RoPE Scaling

`context-window` sets the token capacity available to one sequence. Input,
chat-template tokens, and generated output all consume this capacity.

Increasing that value allocates more KV-cache space, but it does not by itself
give the model reliable long-context behavior. Models using Rotary Position
Embeddings (RoPE) may require a scaling method when the requested context
exceeds the length for which the model was trained.

YaRN is one RoPE scaling method. It applies different interpolation behavior
across RoPE frequencies and adjusts attention scaling. Whether it works, and
which factor to use, depends on the specific model. Do not enable it solely
because a larger KV cache fits in memory.

## 7.2 When to Use YaRN

Use YaRN only when all of the following are true:

- The model's documentation explicitly supports YaRN or compatible RoPE
  scaling.
- The required input plus output exceeds the model's native context.
- The documented extension factor covers the context you need.
- The larger KV cache fits while leaving enough memory for the model and active
  requests.
- Representative long-context tests produce acceptable results.

Avoid YaRN when the workload fits within the native context. llama.cpp uses
static YaRN: the configured scale applies at short positions too and can reduce
quality on shorter prompts. Do not assume that one model family's settings are
valid for another model, even when both use RoPE.

## 7.3 Qwen3 Configuration

The Qwen3-8B model documentation identifies 32,768 tokens as the native context
and reports validation up to 131,072 tokens with YaRN. It recommends matching
the scale to the context actually needed.

For a 2× extension to 65,536 tokens:

```yaml
# ~/.kronk/models/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 65536
  rope-scaling-type: yarn
  rope-freq-scale: 0.5
  yarn-orig-ctx: 32768
```

For a 4× extension to 131,072 tokens:

```yaml
# ~/.kronk/models/model_config.yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 131072
  rope-scaling-type: yarn
  rope-freq-scale: 0.25
  yarn-orig-ctx: 32768
```

`rope-freq-scale` is the raw frequency multiplier, so it is the reciprocal of
the extension factor:

```text
rope-freq-scale = native context / configured context
```

For this model, `0.5` represents a 2× extension and `0.25` represents a 4×
extension. This is equivalent to llama.cpp's `--rope-scale 2` and
`--rope-scale 4`, respectively.

Kronk does not derive `rope-freq-scale` from `context-window`. If the setting is
omitted, llama.cpp uses scaling metadata from the GGUF. That is correct only
when the downloaded GGUF already contains the intended scale. Follow the model
or GGUF provider's documentation rather than copying the Qwen3 values to an
unrelated model.

## 7.4 Scaling Types and Parameters

Kronk accepts three `rope-scaling-type` values:

| Value | Behavior |
| ----- | -------- |
| `none` | Applies no RoPE scaling. It does not prevent a separately configured oversized context window. |
| `linear` | Applies the same interpolation scale across RoPE frequencies. Use only when the model documentation calls for it. |
| `yarn` | Applies YaRN frequency-dependent interpolation and attention scaling. |

The available controls are:

| Setting | Meaning when explicitly configured |
| ------- | ---------------------------------- |
| `rope-freq-base` | Overrides the model's RoPE base frequency. Normally leave this to GGUF metadata. |
| `rope-freq-scale` | Raw frequency multiplier; an extension factor of \(N\) uses \(1/N\) when the model's instructions require explicit scaling. |
| `yarn-orig-ctx` | Original context length used by the YaRN calculation. |
| `yarn-ext-factor` | Mix between interpolation and extrapolation. |
| `yarn-attn-factor` | Attention magnitude scaling. |
| `yarn-beta-fast` | YaRN low correction dimension. |
| `yarn-beta-slow` | YaRN high correction dimension. |

When these settings are omitted, Kronk leaves the llama.cpp or GGUF defaults in
place. An omitted or YAML `null` value does not ask Kronk to calculate a value
from the context ratio. Override the advanced YaRN factors only when the model
provider supplies values or controlled evaluation shows they are needed.

## 7.5 Memory and Concurrency

KV-cache capacity grows approximately linearly with the context window. The
actual size depends on the model architecture, layer count, KV heads, head
dimensions, cache data types, backend, and alignment. With multiple generation
slots, the unified KV pool has capacity based on `context-window × nseq-max`.

If a long-context model does not fit, consider:

- reducing `nseq-max`;
- selecting supported quantized KV-cache types; or
- keeping the KV cache on the CPU with `offload-kqv: false`, at a likely
  performance cost.

Do not rely on fixed memory figures from another model. Use Kronk's hardware
analysis and observe actual memory consumption. See
[Chapter 3](chapter-03-model-configuration.md) for KV-cache configuration and
[Chapter 4](chapter-04-batch-processing.md) for concurrency effects.

## 7.6 Validate Quality

An accepted configuration is not proof that the model can use the entire
context reliably. Test the intended model and GGUF with representative data:

1. Establish baseline quality within the native context.
2. Test retrieval and reasoning at several positions near the target length.
3. Reserve enough context for the expected generated output.
4. Compare short-prompt quality with scaling enabled and disabled.
5. Reduce the extension factor if quality or performance is unacceptable.

Prefer the smallest context and scale that satisfy the workload. For Qwen3-8B,
use the native context when average requests remain within 32,768 tokens, a 2×
configuration for workloads around 65,536, and the documented 4× configuration
only when requests genuinely require it.
