# Chapter 6: YaRN Extended Context

## Table of Contents

- [6.1 Understanding Context Extension](#61-understanding-context-extension)
- [6.2 When to Use YaRN](#62-when-to-use-yarn)
- [6.3 Configuration](#63-configuration)
- [6.4 Scaling Types](#64-scaling-types)
- [6.5 Parameter Reference](#65-parameter-reference)
- [6.6 Model-Specific Examples](#66-model-specific-examples)
- [6.7 Memory Impact](#67-memory-impact)
- [6.8 Quality Considerations](#68-quality-considerations)
- [6.9 Example: Long Document Processing](#69-example-long-document-processing)

---



YaRN (Yet another RoPE extensioN) allows models to handle context windows
beyond their native training length. This is essential for long documents,
extended conversations, and complex agentic workflows.

### 6.1 Understanding Context Extension

Language models are trained with a fixed context length (e.g., 8K, 32K tokens).
RoPE (Rotary Position Embedding) encodes position information, but naive
extension beyond training length causes quality degradation.

YaRN applies frequency-dependent interpolation with attention scaling to
maintain quality at extended lengths.

```
Native Context:     32K tokens (training length)
Extended Context:   131K tokens (4x extension with YaRN)
```

### 6.2 When to Use YaRN

**Good candidates for YaRN:**

- Qwen3 models (trained at 32K, support 131K with YaRN)
- Llama models with RoPE scaling support
- Any model where you need 2-4x the native context

**When NOT to use YaRN:**

- If native context is sufficient for your use case
- Extensions beyond 4x (quality degrades significantly)
- Models without RoPE (older architectures)

### 6.3 Configuration

**Basic YaRN Setup:**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072 # Extended context (131K)
    rope_scaling: yarn # Enable YaRN
```

That's often all you need—Kronk auto-calculates the other YaRN parameters
from the context extension ratio.

**Full Configuration (Advanced):**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn
    rope_freq_base: 1000000 # Model-specific (Qwen3 uses 1M)
    rope_freq_scale: null # Auto-calculate
    yarn_ext_factor: null # Auto-calculate
    yarn_attn_factor: 1.0 # Attention scaling
    yarn_beta_fast: 32.0 # Low correction dimension
    yarn_beta_slow: 1.0 # High correction dimension
    yarn_orig_ctx: 32768 # Original training context
```

### 6.4 Scaling Types

Kronk supports three RoPE scaling methods:

**None (Default)**

```yaml
rope_scaling: none
```

Uses native context length. No scaling applied.

**Linear**

```yaml
rope_scaling: linear
```

Simple linear interpolation. Works but quality degrades faster than YaRN
at high extension ratios.

**YaRN (Recommended)**

```yaml
rope_scaling: yarn
```

Frequency-dependent interpolation with attention scaling. Maintains quality
better at 2-4x extensions.

### 6.5 Parameter Reference

| Parameter          | Default        | Description                                         |
| ------------------ | -------------- | --------------------------------------------------- |
| `rope_scaling`     | none           | Scaling method: `none`, `linear`, `yarn`            |
| `rope_freq_base`   | model default  | Base frequency (10000 for Llama, 1000000 for Qwen3) |
| `rope_freq_scale`  | auto           | Frequency scaling factor                            |
| `yarn_ext_factor`  | auto           | Extrapolation mix factor (0 = disable)              |
| `yarn_attn_factor` | 1.0            | Attention magnitude scaling                         |
| `yarn_beta_fast`   | 32.0           | Low correction dimension                            |
| `yarn_beta_slow`   | 1.0            | High correction dimension                           |
| `yarn_orig_ctx`    | model metadata | Original training context size                      |

### 6.6 Model-Specific Examples

**Qwen3 (32K → 131K)**

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 131072
    rope_scaling: yarn
```

Qwen3 models are specifically designed to support 131K context with YaRN.
The default parameters work well.

**Llama 3 (8K → 32K)**

```yaml
models:
  Llama-3-8B-Q8_0:
    context_window: 32768
    rope_scaling: yarn
    rope_freq_base: 10000
```

4x extension from 8K to 32K is within the recommended range.

### 6.7 Memory Impact

Extended context significantly increases memory requirements:

```
Qwen3-8B with F16 KV cache:

32K context:   ~1.6 GB KV cache
64K context:   ~3.2 GB KV cache
131K context:  ~6.5 GB KV cache
```

**Mitigation strategies:**

1. Use KV cache quantization:

```yaml
cache_type_k: q8_0
cache_type_v: q8_0
```

2. Reduce batch parallelism:

```yaml
n_seq_max: 1 # Fewer concurrent requests
```

3. Keep KV cache on CPU (slower but saves VRAM):

```yaml
offload_kqv: false
```

### 6.8 Quality Considerations

**Extension ratio guidelines:**

- 2x extension: Minimal quality loss
- 3x extension: Slight degradation, usually acceptable
- 4x extension: Noticeable but often usable
- > 4x extension: Not recommended

**Testing your configuration:**

1. Start with a known-good prompt at native context
2. Extend to your target length
3. Compare output quality
4. Adjust if needed (reduce extension or try different parameters)

### 6.9 Example: Long Document Processing

Configuration for processing long documents:

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 65536 # 64K context
    rope_scaling: yarn
    n_batch: 4096 # Larger batch for long prompts
    n_ubatch: 1024
    cache_type_k: q8_0
    cache_type_v: q8_0
    n_seq_max: 1 # Single request (memory intensive)
```

This configuration can process documents up to ~50K tokens while leaving
room for generation.

---
