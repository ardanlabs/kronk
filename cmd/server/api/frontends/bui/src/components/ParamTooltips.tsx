import { useRef, useCallback, type ReactNode } from 'react';

export const PARAM_TOOLTIPS = {
  // Sampling – Generation
  temperature: 'Scales the probability distribution to control randomness. Lower values (e.g. 0.2) make output more focused and deterministic; higher values (e.g. 1.5) increase variety and creativity. Leave empty to use the model default.',
  top_p: 'Nucleus sampling — samples only from the smallest set of tokens whose cumulative probability reaches this value. Lower values (e.g. 0.5) focus on the most likely tokens; 1.0 effectively disables top-p filtering. Works alongside temperature.',
  top_k: 'Limits sampling to the top K most probable tokens at each step. Lower values (e.g. 10) make output more predictable; higher values allow more variety. Set very high to approximate no limit. Leave empty to use the model default.',
  min_p: 'Filters out tokens whose probability is below this fraction of the top token\'s probability. For example, 0.05 removes tokens less than 5% as likely as the best choice. Higher = stricter filtering. 0 disables.',

  // Sampling – Repetition Control
  repeat_penalty: 'Penalizes tokens that appeared in the recent context window (controlled by Repeat Last N). Values above 1.0 discourage repetition; 1.0 means no penalty. Typical range is 1.0–1.3. Too high can make text incoherent.',
  repeat_last_n: 'How many recent tokens to check when applying the repeat penalty. Larger values look further back for repetitions. To disable repetition penalties, set Repeat Penalty to 1.0 instead.',
  frequency_penalty: 'Reduces the likelihood of a token proportional to how many times it has appeared. Positive values discourage overused tokens; negative values encourage them. Common range: -2.0 to 2.0. 0 disables.',
  presence_penalty: 'Applies a flat penalty to any token that has appeared at all, regardless of how often. Positive values encourage the model to use new tokens; negative values favor staying on existing ones. 0 disables.',

  // Sampling – DRY Sampler
  dry_multiplier: 'Strength of the DRY (Don\'t Repeat Yourself) anti-repetition penalty. Higher values more aggressively penalize repeated n-gram patterns. Leave empty to use the model default.',
  dry_base: 'Base for exponential DRY penalty growth. Higher values make the penalty increase faster for longer repeated sequences. Typical values are 1.5–2.0.',
  dry_allowed_length: 'Minimum n-gram length that DRY penalizes. Repeats of length ≥ this value are penalized; shorter repeats are ignored. Useful for allowing common short phrases. Higher values are more lenient.',
  dry_penalty_last_n: 'How many recent tokens DRY examines when looking for repeated patterns. Larger values detect repetitions from further back. 0 means use the full context.',

  // Sampling – XTC Sampler
  xtc_probability: 'Chance of enabling XTC (eXtreme Token Culling) on each token sampling step. When active, XTC removes very high-probability ("obvious") tokens to increase variety. 0 disables XTC entirely, 1 always applies it.',
  xtc_threshold: 'Probability cutoff for XTC culling. When XTC is active, tokens with probability ≥ this threshold are candidates for removal (with safeguards to keep output coherent). Lower thresholds make XTC more aggressive.',
  xtc_min_keep: 'Minimum number of token candidates to keep after XTC culling, preventing over-aggressive filtering. Ensures at least this many choices remain available.',

  // Sampling – Generation limit
  max_tokens: 'Maximum number of tokens (word-pieces) to generate. Rule of thumb: 1 token ≈ 0.75 words in English. Output may stop earlier on end-of-sequence or when the context window is full. Higher values allow longer answers but take more time.',

  // Sampling – Reasoning
  enable_thinking: 'Enables reasoning/thinking mode in the prompt template (model-dependent). May improve accuracy on complex tasks but increases token usage and latency. Some models ignore this or keep reasoning internal.',
  reasoning_effort: 'Requested reasoning level (model/provider dependent): none, minimal, low, medium, high (default: medium). Higher effort may produce more thorough reasoning but uses more tokens and time. Unsupported models ignore this.',

  // Config sweep
  nbatch: 'Batch tray capacity — maximum tokens processed per decode call (shared across slots during prefill). Larger values speed up prompt evaluation and multi-request batching but increase VRAM usage. Typically keep ≤ context window size.',
  nubatch: 'Micro-batch size for prompt processing. Controls VRAM usage per batch operation. Must be ≤ NBatch. Smaller values reduce peak VRAM usage at the cost of slightly slower processing.',
  contextWindow: 'Maximum number of tokens (input + output combined) the model can handle at once. Larger windows support longer conversations but increase VRAM usage proportionally via the KV cache. Models with RoPE can extend beyond native context using YaRN (up to ~4× recommended).',
  nSeqMax: 'Maximum number of concurrent request slots. Each slot handles one user request simultaneously. More slots = better concurrency, but each slot reserves memory for its KV cache.',
  flashAttention: 'Optimized attention algorithm that reduces VRAM usage and can improve speed. "Enabled" forces it on, "Disabled" forces it off, "Auto" lets the server decide based on model compatibility.',
  cacheType: 'KV cache precision. f16 = full precision (best quality), q8_0 = 8-bit quantized (less VRAM, minimal quality loss), q4_0 = 4-bit quantized (most VRAM savings, slight quality trade-off especially at long context).',
  cacheMode: 'Caching strategy (SPC and IMC are mutually exclusive). None = clears KV state after each request. SPC (System Prompt Cache) = reuses cached system-prompt state to speed up new conversations. IMC (Incremental Message Cache) = keeps the conversation\'s KV state in a dedicated slot for fast multi-turn follow-ups.',

  // MoE configuration
  moeMode: 'How to distribute expert weights between GPU and CPU. "Recommended" auto-detects the best option for your hardware. "Save GPU Memory" moves experts to CPU (most common for consumer GPUs). "Maximum Speed" keeps everything on GPU (requires very large VRAM; exact need depends on model, quantization, context, and slots). "Balanced" lets you choose how many layers stay on GPU.',
  moeKeepExpertsTopN: 'Slide right for more speed (keeps more expert layers on GPU), slide left to save VRAM (offloads to CPU). The highest-numbered layers stay on GPU first. 0 = all experts on CPU.',
  moeTipBatch: 'For MoE models with CPU experts, NBatch/NUBatch ≥ 4096 is recommended for optimal prompt processing speed.',
  moeTipFlashAttention: 'Flash Attention is strongly recommended for MoE models — it significantly reduces VRAM usage and improves performance.',
  moeTipComputeBuffer: 'Larger NUBatch increases compute buffer VRAM usage. Monitor with the VRAM calculator when tuning MoE batch sizes.',
  availableVRAM: 'Total GPU VRAM available (in GB). When set, config candidates estimated to exceed this are auto-skipped before the sweep runs. Set to 0 or leave empty to disable VRAM filtering.',

  // NUMA / mmap / Op-offload (Phase F2/F3)
  useMMap: 'Controls whether mmap is used for model loading. Disabling mmap (--no-mmap) is recommended for multi-socket NUMA systems running MoE models with CPU experts — tensor data is directly allocated on the appropriate NUMA node instead of being memory-mapped.',
  numa: 'NUMA (Non-Uniform Memory Access) strategy for multi-socket systems. "distribute" spreads memory across NUMA nodes (recommended for MoE CPU-expert setups). "isolate" pins to one node. "numactl" defers to system numactl. "mirror" mirrors across nodes. Leave empty to disable.',
  opOffloadMinBatch: 'Minimum batch size before enabling GPU offload for certain host-side operations during prompt processing. 0 = use server default. For large MoE models with many CPU weights, values of 200–500+ may improve prompt ingestion speed.',

  // ── Shared VRAM / Config tooltips ──────────────────────────────────────────
  // These are used by both the VRAM Calculator and the Model Playground /
  // Chat settings panels. Keep descriptions hardware-neutral and applicable
  // to both contexts.

  // Controls (shared)
  kvCacheOnCPU: 'Moves the KV cache from GPU VRAM to system RAM. Frees GPU memory but may reduce generation speed — significant on discrete GPUs (PCIe bottleneck), minimal on Apple Silicon (unified memory).',
  gpuCount: 'Number of GPUs to distribute the model across. Weights are split between GPUs using tensor parallelism. More GPUs reduce per-GPU VRAM but add inter-GPU communication overhead.',
  tensorSplit: 'Proportional distribution of model weights across GPUs (e.g. "0.6,0.4" puts 60% on GPU 0 and 40% on GPU 1). Leave empty for equal distribution. Useful when GPUs have different VRAM capacities.',
  expertLayersOnGPU: 'For MoE models: how many transformer block expert layers to keep on GPU. More layers on GPU = faster inference but more VRAM. Layers are kept top-down (highest-numbered first). 0 = all experts on CPU.',
  gpuLayers: 'Number of transformer layers offloaded to GPU (-ngl). All layers on GPU gives maximum speed. Reducing layers saves VRAM by moving weights to system RAM at the cost of inference speed — significant on discrete GPUs (PCIe bottleneck), minimal on Apple Silicon (unified memory).',

  // Model header / metadata
  modelSize: 'Total size of the quantized GGUF model file. For dense models this approximates the GPU VRAM needed for weights alone; for MoE models the actual GPU portion depends on how many expert layers are offloaded to CPU.',
  blockCount: 'Number of transformer blocks (layers) in the model. More layers = larger model. KV cache scales linearly with this value.',
  headCountKV: 'Number of key-value attention heads. Along with key/value lengths, determines the per-token KV cache size. Some models use fewer KV heads than query heads (grouped-query attention) to save memory.',
  keyLength: 'Dimension of each attention key vector (in elements). Together with value length and head count, determines the per-token-per-layer KV cache size.',
  valueLength: 'Dimension of each attention value vector (in elements). Together with key length and head count, determines the per-token-per-layer KV cache size.',
  expertCount: 'Total number of expert sub-networks in a Mixture-of-Experts model. Only a subset (top-k) are activated per token, but all must be stored in memory.',
  activeExperts: 'Number of experts activated per token (top-k routing). Fewer active experts = faster inference per token but the full expert set still occupies memory.',
  sharedExperts: 'Whether the model has shared (always-active) experts in addition to routed ones. Shared experts run on every token and their weights are always loaded on GPU.',

  // VRAM breakdown
  modelWeights: 'GPU memory consumed by the model\'s weight tensors. For dense models this equals the model file size; for MoE models it includes only the always-active weights plus any expert layers kept on GPU.',
  alwaysActiveWeights: 'Memory for non-expert weights that are always loaded on GPU: embeddings, attention layers, normalization, output head, and any shared experts.',
  expertWeightsGPU: 'Memory for the expert layers kept on GPU. Increasing "Expert Layers on GPU" moves more expert blocks from CPU to GPU for faster inference at the cost of VRAM.',
  expertWeightsCPU: 'Memory for expert layers kept in system RAM (CPU-resident). Saves VRAM but typically reduces throughput vs keeping those layers on GPU.',
  kvCache: 'Total KV cache memory across all slots. Formula: slots × context_window × block_count × head_count_kv × (key_length + value_length) × bytes_per_element.',
  kvPerSlot: 'KV cache memory for a single inference slot. Each concurrent request gets its own slot with a full KV cache allocation.',
  kvPerTokenPerLayer: 'KV cache memory for one token in one transformer layer. This is the fundamental unit — total KV = this × context_window × block_count × slots.',
  computeBuffer: 'Estimated temporary GPU memory for scratch/intermediate tensors during inference. This calculator uses a heuristic based on model size and embedding dimensions; actual usage may vary with backend and batch settings.',

  // Hero / summary
  totalEstimatedVRAM: 'Sum of model weights on GPU, KV cache (if on GPU), and estimated compute buffer. In multi-GPU setups this is the total across all GPUs — see the per-GPU breakdown for individual allocations.',
  totalEstimatedSystemRAM: 'Estimated system RAM usage from MoE expert weights on CPU and/or KV cache offloaded to system RAM. Does not include OS or application overhead.',

  // ── Model Card / metadata tooltips ────────────────────────────────────────
  modelArchitecture: 'The neural network architecture family (e.g. llama, qwen2, gemma). Determines how the model processes tokens and which optimizations apply.',
  sizeLabel: 'Human-readable size label from the model publisher (e.g. "8B", "70B"). Indicates the approximate parameter count.',
  quantization: 'The GGUF quantization format used to compress model weights. Lower-bit formats (Q4) save memory at some quality cost; higher-bit formats (Q8, F16) preserve more quality.',
  contextLength: 'The native context window the model was trained on. Input + output tokens must fit within this limit unless extended via YaRN/RoPE scaling.',
  embeddingDimension: 'Size of each token\'s hidden-state vector. Larger embeddings capture more information per token but increase memory and compute proportionally.',
  attentionHeadsQ: 'Number of query attention heads. More heads allow the model to attend to different representation subspaces simultaneously. Together with KV heads, determines the attention pattern.',
  ropeDimension: 'Number of dimensions used for Rotary Position Embeddings (RoPE). Controls how positional information is encoded into attention. Typically equals the head dimension.',
  feedForwardLength: 'Size of the feed-forward (MLP) intermediate layer in each transformer block. Larger values increase model capacity but also VRAM usage.',
  expertFFNLength: 'Size of the feed-forward layer inside each MoE expert sub-network. Similar to Feed Forward Length but specific to the routed expert MLPs.',
  sharedExpertFFNLength: 'Size of the feed-forward layer in shared (always-active) experts. These experts run on every token in addition to the top-k routed experts.',
  ssmInnerSize: 'Dimension of the inner state in the SSM (State Space Model) layers. Larger values increase the model\'s recurrent memory capacity.',
  ssmStateSize: 'Size of the discrete state in SSM layers. Controls how much information the recurrent state can carry between tokens.',
  ssmConvKernel: 'Convolution kernel width in SSM layers. Determines how many neighboring tokens are mixed in the local convolution before the SSM recurrence.',
  ssmTimeStepRank: 'Rank of the time-step projection in SSM layers. Controls the expressiveness of the learned discretization step.',
  ssmGroupCount: 'Number of groups in the SSM layers. Groups partition the state dimensions for more efficient computation, similar to grouped convolution.',
  fullAttentionInterval: 'In hybrid models, how often a full attention layer appears among SSM layers (e.g. every N layers). Balances the efficiency of SSM with the global context of attention.',
  tokenizerModel: 'The tokenizer algorithm used (e.g. BPE, SentencePiece, Unigram). Determines how text is split into tokens that the model processes.',
  eosTokenId: 'End-of-sequence token ID. When the model generates this token, it signals that the response is complete.',
  bosTokenId: 'Beginning-of-sequence token ID. Prepended to the input to signal the start of a new sequence to the model.',
  paddingTokenId: 'Padding token ID used to fill sequences to a uniform length in batch processing. Not used during generation.',

  // ── Model detail / config tooltips ──────────────────────────────────────────
  device: 'Hardware accelerator used for inference. "metal" = Apple GPU, "cuda" = NVIDIA GPU, "vulkan" = cross-platform GPU. "default" lets the server auto-detect.',
  nthreads: 'Number of CPU threads used for inference operations. More threads can speed up CPU-bound work but may cause contention on busy systems. 0 or empty = auto (typically physical core count).',
  nthreadsBatch: 'Number of CPU threads used during prompt (batch) processing. Can differ from inference threads to optimize throughput during the prefill phase. 0 or empty = same as Threads.',
  cacheTypeK: 'Precision format for the key portion of the KV cache. f16 = full precision (best quality), q8_0 = 8-bit quantized (less VRAM, minimal quality loss), q4_0 = 4-bit (most savings).',
  cacheTypeV: 'Precision format for the value portion of the KV cache. Same options as Cache Type K. Some models benefit from asymmetric K/V quantization.',
  cacheMinTokens: 'Minimum token count required before cache reuse kicks in. Higher values avoid caching very short prompts; lower values maximize reuse but can consume more memory for small requests.',
  useDirectIO: 'Uses direct I/O for model file reads, bypassing the OS page cache. Can reduce double-buffering and cache pressure for large model loads, but may be slower or unsupported on some filesystems.',
  ignoreIntegrityCheck: 'Skips model file integrity verification during load. Useful only when you trust the file source and need to bypass a known false positive; otherwise leave disabled to catch corrupted or partial files.',
  offloadKQV: 'Offloads key/query/value attention operations to GPU. Can improve performance on GPU-backed inference but increases VRAM usage.',
  opOffload: 'Allows selected host-side tensor operations to be offloaded to GPU during prompt processing. Can improve throughput for large or CPU-heavy workloads.',
  mainGpu: 'Primary GPU index used in multi-GPU configurations. Relevant when using split mode or explicit device placement. Leave empty on single-GPU systems.',
  devices: 'Explicit list of devices for inference (e.g. CUDA0,CUDA1). Leave empty to let the runtime auto-select.',
  autoFitVram: 'Automatically adjusts GPU-related settings to fit available VRAM. Helpful for avoiding out-of-memory errors, though the chosen config may be more conservative than manual tuning.',
  ropeFreqScale: 'RoPE frequency scale multiplier for context extension. Usually left at the model default unless reproducing a known long-context configuration.',
  yarnBetaFast: 'YaRN beta-fast parameter for short-range frequency transition behavior. Advanced tuning option; usually leave unset unless matching a known config.',
  yarnBetaSlow: 'YaRN beta-slow parameter for long-range frequency transition behavior. Advanced tuning option; usually leave unset unless matching a known config.',
  draftGpuLayers: 'Number of draft-model layers offloaded to GPU. More GPU layers speed up speculative decoding but use more VRAM.',
  draftDevice: 'Device for running the draft model. Useful in multi-device setups when you want the draft model placed separately from the main model.',
  grammar: 'Grammar constraint to force output into a specific syntax (e.g. JSON or a custom GBNF grammar). Improves structured output reliability but can over-constrain generation if the grammar is too strict.',
  ngpuLayers: 'Number of model layers offloaded to GPU. 0 = all layers on GPU (default). -1 = all layers on CPU. Positive N = first N layers on GPU. Lower values save VRAM but reduce speed.',
  splitMode: 'How model weights are distributed across multiple GPUs. "layer" assigns whole layers per GPU, "row" splits individual tensor rows. "none" uses a single GPU.',
  systemPromptCache: 'Caches the KV state of the system prompt so it can be reused across new conversations without re-processing. Saves prefill time when every request shares the same system prompt.',
  incrementalCache: 'Keeps the full conversation KV state in a dedicated slot between requests. Enables fast multi-turn follow-ups by only processing new tokens instead of the entire history.',
  ropeScaling: 'Type of RoPE (Rotary Position Embedding) scaling used to extend the model\'s context window beyond its native training length. "yarn" is the most common method.',
  yarnOrigCtx: 'The model\'s original (native) context length before YaRN extension. YaRN uses this as the baseline to calculate scaling factors. "auto" reads it from the model metadata.',
  ropeFreqBase: 'Base frequency for RoPE position embeddings. Higher values stretch the positional encoding, allowing longer contexts. Typically set automatically by YaRN configuration.',
  yarnExtFactor: 'YaRN extension factor controlling how aggressively context is extended. -1 = auto-calculate based on context ratio. Higher values extend more but may reduce quality.',
  yarnAttnFactor: 'YaRN attention scaling factor that adjusts attention weight magnitude at extended positions. Fine-tunes quality at long context lengths.',
  draftModel: 'Speculative decoding draft model — a smaller, faster model that proposes candidate tokens which the main model then verifies. Speeds up generation when acceptance rate is high.',
  draftTokens: 'Number of tokens the draft model proposes per speculative decoding step. More tokens = potentially faster generation, but too many reduces acceptance rate.',
  hasProjection: 'Whether the model includes a multi-modal projection file (mmproj). Required for vision or audio input — the projection maps image/audio embeddings into the model\'s token space.',
  isGPT: 'Whether the model uses a GPT-style (causal, decoder-only) architecture. GPT models generate text left-to-right. Non-GPT models may be encoder-decoder or embedding models.',
  validated: 'Whether the model has been validated against the Kronk catalog. Validated models have confirmed-working configurations, templates, and recommended settings.',
} as const satisfies Record<string, string>;

export type TooltipKey = keyof typeof PARAM_TOOLTIPS;

type ParamTooltipProps =
  | { tooltipKey: TooltipKey; text?: never }
  | { text: string; tooltipKey?: never };

export function ParamTooltip(props: ParamTooltipProps) {
  const text = props.tooltipKey ? PARAM_TOOLTIPS[props.tooltipKey] : props.text!;
  const wrapperRef = useRef<HTMLSpanElement>(null);
  const tipRef = useRef<HTMLSpanElement>(null);

  const reposition = useCallback(() => {
    const wrapper = wrapperRef.current;
    const tip = tipRef.current;
    if (!wrapper || !tip) return;
    const iconRect = wrapper.getBoundingClientRect();
    const tipWidth = tip.offsetWidth;
    const tipHeight = tip.offsetHeight;

    // Position above the icon using viewport coordinates (fixed positioning).
    const top = iconRect.top - tipHeight - 8;

    // Align left edge of tooltip with the icon, then clamp to viewport.
    let left = iconRect.left;
    const rightOverflow = left + tipWidth - window.innerWidth + 8;
    if (rightOverflow > 0) {
      left -= rightOverflow;
    }
    if (left < 8) {
      left = 8;
    }
    tip.style.left = `${left}px`;
    tip.style.top = `${top}px`;

    // Position the arrow to point at the icon.
    const arrowLeft = Math.max(10, Math.min(tipWidth - 10, iconRect.left - left + iconRect.width / 2));
    tip.style.setProperty('--arrow-left', `${arrowLeft}px`);
  }, []);

  return (
    <span className="param-tooltip-wrapper" ref={wrapperRef} onMouseEnter={reposition}>
      <span className="param-tooltip-icon">ⓘ</span>
      <span className="param-tooltip-text" ref={tipRef}>{text}</span>
    </span>
  );
}

export function labelWithTip(label: string, tooltipKey: TooltipKey): ReactNode {
  return <>{label} <ParamTooltip tooltipKey={tooltipKey} /></>;
}

type FieldLabelProps = React.LabelHTMLAttributes<HTMLLabelElement> & {
  children: ReactNode;
  tooltipKey?: TooltipKey;
  after?: ReactNode;
};

export function FieldLabel({ children, tooltipKey, after, ...props }: FieldLabelProps) {
  return (
    <label {...props}>
      {children}
      {tooltipKey && <ParamTooltip tooltipKey={tooltipKey} />}
      {after}
    </label>
  );
}
