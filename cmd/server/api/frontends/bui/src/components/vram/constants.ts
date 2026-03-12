export const CONTEXT_WINDOW_OPTIONS = [
  { value: 1024, label: '1K' },
  { value: 2048, label: '2K' },
  { value: 4096, label: '4K' },
  { value: 8192, label: '8K' },
  { value: 16384, label: '16K' },
  { value: 32768, label: '32K' },
  { value: 65536, label: '64K' },
  { value: 98304, label: '96K' },
  { value: 131072, label: '128K' },
  { value: 262144, label: '256K' },
  { value: 524288, label: '512K' },
  { value: 1048576, label: '1M' },
];

export const BYTES_PER_ELEMENT_OPTIONS = [
  { value: 1, label: 'q8_0 / q4_0 / q4_1 / q5_0 / q5_1 (1 byte)' },
  { value: 2, label: 'f16 / bf16 (2 bytes)' },
  { value: 4, label: 'f32 (4 bytes)' },
];

export const SLOT_OPTIONS = [1, 2, 3, 4, 5];

export const VRAM_FORMULA_CONTENT = `VRAM CALCULATION FORMULA

Total VRAM ≈ Model Weights (GPU) + KV Cache (if on GPU) + Compute Buffer

Model weights are determined by the GGUF file size (e.g., ~8GB for a
7B Q8_0 model). The KV cache is the variable cost you control through
configuration. The compute buffer is a heuristic estimate of scratch
memory needed during inference.

==============================================================================
SLOTS AND SEQUENCES
==============================================================================

A slot is a processing unit that handles one request at a time. Each slot
is assigned a unique sequence ID that maps to an isolated partition in the
shared KV cache. The mapping is always 1:1:

  NSeqMax = 4 (set via n_seq_max in model config)

  Slot 0  →  Sequence 0  →  KV cache partition 0
  Slot 1  →  Sequence 1  →  KV cache partition 1
  Slot 2  →  Sequence 2  →  KV cache partition 2
  Slot 3  →  Sequence 3  →  KV cache partition 3

NSeqMax controls how many slots (and sequences) are created. More slots
means more concurrent requests, but each slot reserves its own KV cache
partition in VRAM whether or not it is actively used.

==============================================================================
WHAT AFFECTS KV CACHE MEMORY PER SEQUENCE
==============================================================================

Each sequence's KV cache partition size is determined by three factors:

  1. Context Window (n_ctx)
     The maximum number of tokens the sequence can hold. Larger context
     windows linearly increase memory. 32K context uses 4× the memory
     of 8K context.

  2. Number of Layers (block_count)
     Every transformer layer stores its own key and value tensors per
     token. More layers means more memory per token. A 70B model with
     80 layers uses ~2.5× more per-token memory than a 7B model with
     32 layers.

  3. KV Cache Precision (bytes_per_element)
     The data type used to store cached keys and values:
       f16  = 2 bytes per element (default, best quality)
       q8_0 = 1 byte per element  (50% VRAM savings, good quality)
     The head geometry (head_count_kv, key_length, value_length) is
     fixed by the model architecture and read from the GGUF header.

The formula:

  KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
  KV_Per_Sequence        = n_ctx × n_layers × KV_Per_Token_Per_Layer

==============================================================================
WHAT AFFECTS TOTAL KV CACHE (SLOT MEMORY)
==============================================================================

Total KV cache (Slot Memory) is simply the per-sequence cost multiplied
by the number of slots:

  Slot_Memory = NSeqMax × KV_Per_Sequence
  Total_VRAM  = Model_Weights + Slot_Memory

Memory is statically allocated upfront when the model loads. All slots
reserve their full KV cache partition regardless of whether they are
actively processing a request.

==============================================================================
KV CACHE ON CPU (offload-kqv: false)
==============================================================================

When "KV Cache on CPU" is enabled, the KV cache is stored in system RAM
instead of GPU VRAM:

  Total_VRAM       = Model_Weights + Compute_Buffer   (no KV cache)
  System_RAM_Used  = KV_Cache + CPU_Weights (if any MoE experts on CPU)

Performance implications:
  - Discrete GPUs (CUDA/ROCm/Vulkan): Every token generation requires
    reading the entire KV cache across the PCIe bus. Expect 2-5× slower
    token generation depending on bus bandwidth.
  - Apple Silicon (Metal): Minimal penalty due to unified memory — CPU
    and GPU share the same memory pool, so no data transfer is needed.
  - Prompt processing (prefill) is less affected since it is compute-bound.

Use this when VRAM is insufficient for the KV cache and you need a larger
context window or more concurrent slots. KV cache quantization (q8_0) is
often a better first step as it halves KV memory with minimal quality loss.

==============================================================================
EXAMPLE: REAL MODEL CALCULATION
==============================================================================

Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
Model Weights           : 36.0 GB
Context Window (n_ctx)  : 131,072 (128K)
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 48
attention.head_count_kv : 4
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 4 × (128 + 128) × 1 = 1,024 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 131,072 × 48 × 1,024 = ~6.4 GB

Step 3 — Total KV cache (NSeqMax = 2):

  Slot_Memory = 2 × 6.4 GB = ~12.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 36.0 GB + 12.8 GB = ~48.8 GB

==============================================================================
MULTI-GPU VRAM DISTRIBUTION
==============================================================================

When using multiple GPUs, model weights and KV cache are distributed
across devices according to the tensor split configuration:

  tensor-split: [0.6, 0.4]   # 60% on GPU 0, 40% on GPU 1

Split modes control distribution strategy:
  - none:  All on single GPU (MainGPU)
  - layer: Split layers across GPUs
  - row:   Tensor parallelism (recommended for MoE, expert-parallel)

For MoE with CPU expert offload:
  - Single GPU + CPU experts: split-mode: row (simpler, good default)
  - Multi-GPU + CPU experts: split-mode: layer may be simpler;
    row can interact with CPU expert offload in surprising ways

Compute buffer memory is primarily allocated on the main GPU (GPU 0).

Note: Per-GPU estimates assume proportional distribution based on
tensor_split. Actual allocation by llama.cpp may differ slightly.`;
