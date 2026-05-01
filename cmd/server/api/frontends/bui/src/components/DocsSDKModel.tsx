import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKModel() {
  const location = useLocation();

  useEffect(() => {
    const container = document.querySelector('.main-content');
    if (!container) return;
    if (!location.hash) {
      container.scrollTo({ top: 0 });
      return;
    }
    const id = location.hash.slice(1);
    requestAnimationFrame(() => {
      const element = document.getElementById(id);
      if (!element) return;
      const containerRect = container.getBoundingClientRect();
      const elementRect = element.getBoundingClientRect();
      const offset = elementRect.top - containerRect.top + container.scrollTop;
      container.scrollTo({ top: offset - 20, behavior: 'smooth' });
    });
  }, [location.key, location.hash]);

  return (
    <div>
      <div className="page-header">
        <h2>Model Package</h2>
        <p>Package model provides the low-level api for working with models.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/kronk/model"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-addparams">
              <h4>AddParams</h4>
              <pre className="code-block">
                <code>func AddParams(params Params, d D)</code>
              </pre>
              <p className="doc-description">AddParams adds the values from the Params struct into the provided D map. Only non-zero values are added.</p>
            </div>

            <div className="doc-section" id="func-checkmodel">
              <h4>CheckModel</h4>
              <pre className="code-block">
                <code>func CheckModel(modelFile string, checkSHA bool) error</code>
              </pre>
              <p className="doc-description">CheckModel is check if the downloaded model is valid based on it's sha file. If no sha file exists, this check will return with no error.</p>
            </div>

            <div className="doc-section" id="func-inityzmaworkarounds">
              <h4>InitYzmaWorkarounds</h4>
              <pre className="code-block">
                <code>func InitYzmaWorkarounds(libPath string) error</code>
              </pre>
              <p className="doc-description">InitYzmaWorkarounds loads the mtmd library and preps our fixed FFI functions. This is safe to call multiple times; it only initializes once.</p>
            </div>

            <div className="doc-section" id="func-newgrammarsampler">
              <h4>NewGrammarSampler</h4>
              <pre className="code-block">
                <code>func NewGrammarSampler(vocab llama.Vocab, grammar string) *grammarSampler</code>
              </pre>
              <p className="doc-description">NewGrammarSampler creates a grammar sampler that will be managed separately from the main sampler chain.</p>
            </div>

            <div className="doc-section" id="func-parseggmltype">
              <h4>ParseGGMLType</h4>
              <pre className="code-block">
                <code>func ParseGGMLType(s string) (GGMLType, error)</code>
              </pre>
              <p className="doc-description">ParseGGMLType parses a string into a GGMLType. Supported values: "f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "bf16", "auto".</p>
            </div>

            <div className="doc-section" id="func-newmodel">
              <h4>NewModel</h4>
              <pre className="code-block">
                <code>func NewModel(ctx context.Context, cfg Config) (*Model, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="func-detectmodeltypefromfiles">
              <h4>DetectModelTypeFromFiles</h4>
              <pre className="code-block">
                <code>func DetectModelTypeFromFiles(modelFiles []string) (ModelType, string, error)</code>
              </pre>
              <p className="doc-description">DetectModelTypeFromFiles loads a model from the given GGUF files, determines the architecture type, and immediately frees the model. It returns the ModelType, the raw general.architecture string from the GGUF metadata, and any error encountered during loading.</p>
            </div>

            <div className="doc-section" id="func-parseropescalingtype">
              <h4>ParseRopeScalingType</h4>
              <pre className="code-block">
                <code>func ParseRopeScalingType(s string) (RopeScalingType, error)</code>
              </pre>
              <p className="doc-description">ParseRopeScalingType parses a string into a RopeScalingType. Supported values: "none", "linear", "yarn".</p>
            </div>

            <div className="doc-section" id="func-parsesplitmode">
              <h4>ParseSplitMode</h4>
              <pre className="code-block">
                <code>func ParseSplitMode(s string) (SplitMode, error)</code>
              </pre>
              <p className="doc-description">ParseSplitMode parses a string into a SplitMode. Supported values: "none", "layer", "row", "expert-parallel", "tensor-parallel".</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-chatresponse">
              <h4>ChatResponse</h4>
              <pre className="code-block">
                <code>{`type ChatResponse struct {
	ID                string   \`json:"id"\`
	Object            string   \`json:"object"\`
	Created           int64    \`json:"created"\`
	Model             string   \`json:"model"\`
	SystemFingerprint string   \`json:"system_fingerprint"\`
	Choices           []Choice \`json:"choices"\`
	Usage             *Usage   \`json:"usage,omitempty"\`
	Prompt            string   \`json:"prompt,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">ChatResponse represents output for inference models.</p>
            </div>

            <div className="doc-section" id="type-choice">
              <h4>Choice</h4>
              <pre className="code-block">
                <code>{`type Choice struct {
	Index           int              \`json:"index"\`
	Message         *ResponseMessage \`json:"message,omitempty"\`
	Delta           *ResponseMessage \`json:"delta,omitempty"\`
	Logprobs        *Logprobs        \`json:"logprobs,omitempty"\`
	FinishReasonPtr *string          \`json:"finish_reason"\`
}`}</code>
              </pre>
              <p className="doc-description">Choice represents a single choice in a response.</p>
            </div>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	PtrCacheMinTokens    *int
	PtrCacheSlotTimeout  *int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	PtrContextWindow     *int
	DefaultParams        Params
	DraftModel           *DraftModelConfig
	Devices              []string // Device names for model execution (e.g., ["CUDA0", "CUDA1"])
	FlashAttention       FlashAttentionType
	PtrIncrementalCache  *bool
	PtrInsecureLogging   *bool
	JinjaFile            string
	Log                  applog.Logger
	PtrMainGPU           *int
	MoE                  *MoEConfig
	ModelFiles           []string
	PtrNBatch            *int
	PtrNGpuLayers        *int
	PtrNSeqMax           *int
	PtrNThreads          *int
	PtrNThreadsBatch     *int
	PtrNUBatch           *int
	NUMA                 string
	PtrOffloadKQV        *bool
	PtrOpOffload         *bool
	PtrOpOffloadMinBatch *int
	ProjFile             string
	PtrQueueDepth        *int
	PtrRopeFreqBase      *float32
	PtrRopeFreqScale     *float32
	RopeScaling          RopeScalingType
	PtrSplitMode         *SplitMode
	PtrSWAFull           *bool
	TensorBuftOverrides  []string
	TensorSplit          []float32
	PtrUseDirectIO       *bool
	PtrUseMMap           *bool
	PtrYarnAttnFactor    *float32
	PtrYarnBetaFast      *float32
	PtrYarnBetaSlow      *float32
	PtrYarnExtFactor     *float32
	PtrYarnOrigCtx       *int
}`}</code>
              </pre>
              <p className="doc-description">Config represents model level configuration. These values if configured incorrectly can cause the system to panic. The defaults are used when these values are set to 0. CacheMinTokens sets the minimum token count required before caching. Messages shorter than this threshold are not cached, as the overhead of cache management may outweigh the prefill savings. When set to 0, defaults to 100 tokens. CacheSlotTimeout sets the maximum number of seconds used for two IMC timeout scenarios: 1. Wait for slot availability: When all IMC slots have cache builds in-flight (pending), incoming requests wait up to this duration for a slot to become available before returning a "server busy" error. 2. Slot preemption: When a request's target slot is busy generating, the request is deferred. If the deferred job waits longer than this duration (measured from batch queue entry, not HTTP arrival), the busy slot is preempted so the waiting request can proceed. When set to 0, defaults to 30 seconds. CacheTypeK is the data type for the K (key) cache. This controls the precision of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0) reduce memory usage but may slightly affect quality. When left as the zero value (GGMLTypeAuto), the default llama.cpp value is used. CacheTypeV is the data type for the V (value) cache. This controls the precision of the value vectors in the KV cache. When left as the zero value (GGMLTypeAuto), the default llama.cpp value is used. ContextWindow (often referred to as context length) is the maximum number of tokens that a large language model can process and consider at one time when generating a response. It defines the model's effective "memory" for a single conversation or text generation task. When set to 0, the default value is 4096. DefaultParams contains the default sampling parameters for requests. Devices is a list of device names to use for model execution. When multiple devices are specified, the model is distributed across them according to the SplitMode and TensorSplit configuration. Device names can be obtained from the output of llama-bench --list-devices (e.g., "CUDA0", "CUDA1", "Metal"). When empty and Device is also empty, the default device selection is used. FlashAttention controls Flash Attention mode. Flash Attention reduces memory usage and speeds up attention computation, especially for large context windows. When left as zero value, FlashAttentionEnabled is used (default on). Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide. IncrementalCache enables Incremental Message Caching (IMC) for agentic workflows. It caches all messages except the last one (which triggers generation) and extends the cache incrementally on each turn. This is ideal for agents like Cline or OpenCode where conversations grow monotonically. The cache is rebuilt from scratch when the message prefix changes (new thread). InsecureLogging enables logging of potentially sensitive data such as message content. This should only be enabled for debugging purposes in non-production environments. JinjaFile is the path to the jinja file. This is not required and can be used if you want to override the templated provided by the model metadata. Log is the logger to use for model operations. ModelFiles is the path to the model files. This is mandatory to provide. NBatch is the logical batch size or the maximum number of tokens that can be in a single forward pass through the model at any given time. It defines the maximum capacity of the processing batch. If you are processing a very long prompt or multiple prompts simultaneously, the total number of tokens processed in one go will not exceed NBatch. Increasing n_batch can improve performance (throughput) if your hardware can handle it, as it better utilizes parallel computation. However, a very high n_batch can lead to out-of-memory errors on systems with limited VRAM. When set to 0, the default value is 2048. MainGPU is the index of the GPU to use as the primary device when SplitMode is SplitModeNone. When nil, the default GPU (usually index 0) is used. NGpuLayers is the number of model layers to offload to the GPU. When set to 0, all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any positive value specifies the exact number of layers to offload. NSeqMax controls concurrency behavior based on model type. For text inference models (including vision/audio), it sets the maximum number of sequences processed in parallel within the batch engine. For embedding and reranking models, it sets the number of contexts in the internal pool for parallel request processing. When set to 0, a default of 1 is used. NThreads is the number of threads to use for generation. When set to 0, the default llama.cpp value is used. NThreadsBatch is the number of threads to use for batch processing. When set to 0, the default llama.cpp value is used. NUBatch is the physical batch size or the maximum number of tokens processed together during the initial prompt processing phase (also called "prompt ingestion") to populate the KV cache. It specifically optimizes the initial loading of prompt tokens into the KV cache. If a prompt is longer than NUBatch, it will be broken down and processed in chunks of n_ubatch tokens sequentially. This parameter is crucial for tuning performance on specific hardware (especially GPUs) because different values might yield better prompt processing times depending on the memory architecture. When set to 0, the default value is 512. OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or true, the KV cache is stored on the GPU (default behavior). Set to false to keep the KV cache on the CPU, which reduces VRAM usage but may slow inference. OpOffload controls whether host tensor operations are offloaded to the device (GPU). When nil or true, operations are offloaded (default behavior). Set to false to keep operations on the CPU. ProjFile is the path to the projection files. This is mandatory for media based models like vision and audio. QueueDepth sets the multiplier for semaphore capacity when using the batch engine (NSeqMax &gt; 1). This controls how many requests can queue while the current batch is processing. Default is 2, meaning NSeqMax * 2 requests can be in-flight. Only applies to text inference models. RopeFreqBase overrides the RoPE base frequency. When nil, uses model default. Common values: 10000 (Llama), 1000000 (Qwen3). RopeFreqScale overrides the RoPE frequency scaling factor. When nil, uses model default or auto-calculates based on context extension ratio. RopeScaling controls the RoPE scaling method for extended context support. Set to RopeScalingYaRN to enable YaRN scaling for models like Qwen3 that support extended context (e.g., 32k training → 131k with YaRN). SWAFull controls whether models with sliding window attention (SWA) use a full-size KV cache for SWA layers instead of the memory-efficient small cache. When nil (default), llama.cpp's default is used (currently true). When explicitly set to false, SWA layers only cache the last n_swa tokens, saving significant VRAM but limiting context caching and shifting. When true, SWA layers use the full context window for their KV cache, preserving accuracy at the cost of higher memory usage. SplitMode controls how the model is split across multiple GPUs: - SplitModeNone (0): single GPU - SplitModeLayer (1): split layers and KV across GPUs - SplitModeRow (2): split layers and KV across GPUs with tensor parallelism (recommended for MoE models like Qwen3-MoE, Mixtral, DeepSeek) When nil (not set), defaults to SplitModeRow for optimal MoE performance. TensorBuftOverrides is a list of tensor buffer type override patterns that force matching tensors to execute on CPU instead of GPU. This is an expert-level configuration useful for MoE models where certain FFN expert tensors don't fit in VRAM. Supported values: - "all-ffn": offload all FFN expression tensors to CPU - "block:N": offload FFN tensors for block N to CPU (e.g., "block:12") - Any regex pattern matching tensor names (e.g., `blk\.12\.ffn_(up|down|gate)`) TensorSplit controls how model layers are proportionally distributed across multiple GPUs. Each element represents the fraction of the model assigned to the corresponding device. For example, [0.6, 0.4] splits 60%/40% across two GPUs. The length must match the number of devices. When empty, the split is determined automatically based on available VRAM. UseDirectIO enables direct I/O for model loading. UseMMap controls whether mmap is used for model loading. When nil, mmap is enabled by default (llama.cpp default). Set to false to disable mmap, which is recommended for multi-socket NUMA systems running MoE models with CPU experts — without mmap, tensor data is directly allocated and can be placed on the appropriate NUMA node. UseDirectIO takes precedence over UseMMap. NUMA controls the NUMA (Non-Uniform Memory Access) strategy. This matters most when expert tensors are on CPU and the system has multiple NUMA nodes. Valid values: "" (disabled), "distribute", "isolate", "numactl", "mirror". "distribute" is recommended for multi-socket MoE setups; without it, cross-socket memory access can cause significant bandwidth collapse. YarnAttnFactor sets the YaRN attention magnitude scaling factor. When nil, uses default of 1.0. YarnBetaFast sets the YaRN low correction dimension. When nil, uses default of 32.0. YarnBetaSlow sets the YaRN high correction dimension. When nil, uses default of 1.0. YarnExtFactor sets the YaRN extrapolation mix factor. When nil, auto-calculated from context scaling ratio. Set to 0 to disable extrapolation. YarnOrigCtx sets the original training context size for YaRN scaling. When nil or 0, uses the model's native training context length from metadata.</p>
            </div>

            <div className="doc-section" id="type-contentlogprob">
              <h4>ContentLogprob</h4>
              <pre className="code-block">
                <code>{`type ContentLogprob struct {
	Token       string       \`json:"token"\`
	Logprob     float32      \`json:"logprob"\`
	Bytes       []byte       \`json:"bytes,omitempty"\`
	TopLogprobs []TopLogprob \`json:"top_logprobs,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">ContentLogprob represents log probability information for a single token.</p>
            </div>

            <div className="doc-section" id="type-d">
              <h4>D</h4>
              <pre className="code-block">
                <code>{`type D map[string]any`}</code>
              </pre>
              <p className="doc-description">D represents a generic docment of fields and values.</p>
            </div>

            <div className="doc-section" id="type-draftmodelconfig">
              <h4>DraftModelConfig</h4>
              <pre className="code-block">
                <code>{`type DraftModelConfig struct {
	ModelFiles    []string  // Path to the draft model GGUF file(s)
	NDraft        int       // Number of tokens to draft per step (default 5)
	PtrNGpuLayers *int      // GPU layers for draft model (nil = all layers on GPU)
	Devices       []string  // Devices for draft model (e.g., ["CUDA0"])
	PtrMainGPU    *int      // Primary GPU index for draft model
	TensorSplit   []float32 // Per-device tensor split for draft model
}`}</code>
              </pre>
              <p className="doc-description">DraftModelConfig configures a draft model for speculative decoding. A smaller, faster model generates candidate tokens that the target model verifies in a single forward pass. This can improve generation throughput when the draft model's predictions frequently match the target's. Requirements: - Draft and target models must share the same vocabulary (same tokenizer) - NSeqMax must be 1 (single-slot mode) - Draft model should be significantly smaller than the target (e.g., 0.6B draft for 8B target)</p>
            </div>

            <div className="doc-section" id="type-embeddata">
              <h4>EmbedData</h4>
              <pre className="code-block">
                <code>{`type EmbedData struct {
	Object    string    \`json:"object"\`
	Index     int       \`json:"index"\`
	Embedding []float32 \`json:"embedding"\`
}`}</code>
              </pre>
              <p className="doc-description">EmbedData represents the data associated with an embedding call.</p>
            </div>

            <div className="doc-section" id="type-embedreponse">
              <h4>EmbedReponse</h4>
              <pre className="code-block">
                <code>{`type EmbedReponse struct {
	Object  string      \`json:"object"\`
	Created int64       \`json:"created"\`
	Model   string      \`json:"model"\`
	Data    []EmbedData \`json:"data"\`
	Usage   EmbedUsage  \`json:"usage"\`
}`}</code>
              </pre>
              <p className="doc-description">EmbedReponse represents the output for an embedding call.</p>
            </div>

            <div className="doc-section" id="type-embedusage">
              <h4>EmbedUsage</h4>
              <pre className="code-block">
                <code>{`type EmbedUsage struct {
	PromptTokens int \`json:"prompt_tokens"\`
	TotalTokens  int \`json:"total_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">EmbedUsage provides token usage information for embeddings.</p>
            </div>

            <div className="doc-section" id="type-flashattentiontype">
              <h4>FlashAttentionType</h4>
              <pre className="code-block">
                <code>{`type FlashAttentionType int32`}</code>
              </pre>
              <p className="doc-description">FlashAttentionType controls when to enable Flash Attention. Flash Attention reduces memory usage and speeds up attention computation, especially beneficial for large context windows.</p>
            </div>

            <div className="doc-section" id="type-ggmltype">
              <h4>GGMLType</h4>
              <pre className="code-block">
                <code>{`type GGMLType int32`}</code>
              </pre>
              <p className="doc-description">GGMLType represents a ggml data type for the KV cache. These values correspond to the ggml_type enum in llama.cpp.</p>
            </div>

            <div className="doc-section" id="type-logger">
              <h4>Logger</h4>
              <pre className="code-block">
                <code>{`type Logger = applog.Logger`}</code>
              </pre>
              <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
            </div>

            <div className="doc-section" id="type-logprobs">
              <h4>Logprobs</h4>
              <pre className="code-block">
                <code>{`type Logprobs struct {
	Content []ContentLogprob \`json:"content,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">Logprobs contains log probability information for the response.</p>
            </div>

            <div className="doc-section" id="type-mediatype">
              <h4>MediaType</h4>
              <pre className="code-block">
                <code>{`type MediaType int`}</code>
              </pre>
            </div>

            <div className="doc-section" id="type-moeconfig">
              <h4>MoEConfig</h4>
              <pre className="code-block">
                <code>{`type MoEConfig struct {
	// Mode controls expert placement strategy.
	Mode MoEMode \`yaml:"mode,omitempty"\`

	// PtrKeepExpertsOnGPUForTopNLayers keeps routed expert tensors on GPU for the
	// top N layers (highest-index layers). All other expert layers go to CPU.
	// Only used when Mode is MoEModeKeepTopN. 0 means all experts on CPU.
	// llama.cpp convention: "top" means highest-numbered layers.
	PtrKeepExpertsOnGPUForTopNLayers *int \`yaml:"keep-experts-top-n,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">MoEConfig configures Mixture of Experts tensor placement. When nil, no MoE-specific behavior is applied.</p>
            </div>

            <div className="doc-section" id="type-moemode">
              <h4>MoEMode</h4>
              <pre className="code-block">
                <code>{`type MoEMode string`}</code>
              </pre>
              <p className="doc-description">MoEMode controls expert placement strategy for Mixture of Experts models.</p>
            </div>

            <div className="doc-section" id="type-model">
              <h4>Model</h4>
              <pre className="code-block">
                <code>{`type Model struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Model represents a model and provides a low-level API for working with it.</p>
            </div>

            <div className="doc-section" id="type-modelinfo">
              <h4>ModelInfo</h4>
              <pre className="code-block">
                <code>{`type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	VRAMTotal     int64
	SlotMemory    int64
	Type          ModelType
	IsGPTModel    bool
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
	Template      Template
}`}</code>
              </pre>
              <p className="doc-description">ModelInfo represents the model's card information.</p>
            </div>

            <div className="doc-section" id="type-modeltype">
              <h4>ModelType</h4>
              <pre className="code-block">
                <code>{`type ModelType uint8`}</code>
              </pre>
              <p className="doc-description">ModelType represents the model architecture for batch engine state management.</p>
            </div>

            <div className="doc-section" id="type-option">
              <h4>Option</h4>
              <pre className="code-block">
                <code>{`type Option func(*Config)`}</code>
              </pre>
              <p className="doc-description">Option represents a functional option for configuring a Config.</p>
            </div>

            <div className="doc-section" id="type-params">
              <h4>Params</h4>
              <pre className="code-block">
                <code>{`type Params struct {
	// AdaptivePDecay controls how quickly the Adaptive-P sampler adjusts.
	// Default is 0.0.
	AdaptivePDecay float32 \`json:"adaptive_p_decay"\`

	// AdaptivePTarget is the target probability threshold for Adaptive-P
	// sampling. When > 0, enables adaptive sampling that dynamically adjusts
	// based on the probability distribution to prevent predictable patterns.
	// Default is 0.0 (disabled).
	AdaptivePTarget float32 \`json:"adaptive_p_target"\`

	// DryAllowedLen is the minimum n-gram length before DRY applies. Default is 2.
	DryAllowedLen int32 \`json:"dry_allowed_length"\`

	// DryBase is the base for exponential penalty growth in DRY. Default is 1.75.
	DryBase float32 \`json:"dry_base"\`

	// DryMultiplier controls the DRY (Don't Repeat Yourself) sampler which
	// penalizes n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	// Default is 1.05.
	DryMultiplier float32 \`json:"dry_multiplier"\`

	// DryPenaltyLast limits how many recent tokens DRY considers. Default of 0
	// means full context.
	DryPenaltyLast int32 \`json:"dry_penalty_last_n"\`

	// FrequencyPenalty penalizes tokens proportionally to how often they have
	// appeared in the output. Higher values more strongly discourage frequent
	// repetition. Default is 0.0 (disabled).
	FrequencyPenalty float32 \`json:"frequency_penalty"\`

	// Grammar constrains output to match a GBNF grammar specification.
	// When set, the model output will be forced to conform to this grammar.
	// Use preset grammars like GrammarJSON or generate from JSON Schema.
	Grammar string \`json:"grammar"\`

	// IncludeUsage determines whether to include token usage information in
	// streaming responses. Default is true.
	IncludeUsage bool \`json:"include_usage"\`

	// Logprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated
	// token. Default is false.
	Logprobs bool \`json:"logprobs"\`

	// MaxTokens is the maximum tokens for generation when not derived from the
	// model's context window the default is 4096.
	MaxTokens int \`json:"max_tokens"\`

	// MinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text. Default is 0.0.
	MinP float32 \`json:"min_p"\`

	// PresencePenalty applies a flat penalty to any token that has already
	// appeared in the output, regardless of frequency. Higher values encourage
	// the model to introduce new topics. Default is 0.0 (disabled).
	PresencePenalty float32 \`json:"presence_penalty"\`

	// ReasoningEffort is a string that specifies the level of reasoning effort
	// to use for GPT models. Default is ReasoningEffortMedium.
	ReasoningEffort string \`json:"reasoning_effort"\`

	// RepeatLastN specifies how many recent tokens to consider when applying
	// the repetition penalty. A larger value considers more context but may be
	// slower. Default is 64.
	RepeatLastN int32 \`json:"repeat_last_n"\`

	// RepeatPenalty applies a penalty to tokens that have already appeared in
	// the output, reducing repetitive text. A value of 1.0 means no penalty.
	// Values above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is
	// strong). Default is 1.0 which turns it off.
	RepeatPenalty float32 \`json:"repeat_penalty"\`

	// ReturnPrompt determines whether to include the prompt in the final
	// response. When set to true, the prompt will be included. Default is false.
	ReturnPrompt bool \`json:"return_prompt"\`

	// Stream determines whether to stream the response.
	Stream bool \`json:"stream"\`

	// Temperature controls the randomness of the output. It rescales the
	// probability distribution of possible next tokens. Default is 0.8.
	Temperature float32 \`json:"temperature"\`

	// Thinking determines if the model should think or not. It is used for most
	// non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False. Default is "true".
	Thinking string \`json:"enable_thinking"\`

	// TopK limits the pool of possible next tokens to the K number of most
	// probable tokens. If a model predicts 10,000 possible next tokens, setting
	// top_k to 50 means only the 50 tokens with the highest probabilities are
	// considered for selection (after temperature scaling). Default is 40.
	TopK int32 \`json:"top_k"\`

	// TopLogprobs specifies how many of the most likely tokens to return at
	// each position, along with their log probabilities. Must be between 0 and
	// 5. Setting this to a value > 0 implicitly enables logprobs. Default is 0.
	TopLogprobs int \`json:"top_logprobs"\`

	// TopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the
	// minimum number of most probable tokens required to reach the cumulative
	// probability P. Default is 0.9.
	TopP float32 \`json:"top_p"\`

	// XtcMinKeep is the minimum tokens to keep after XTC culling. Default is 1.
	XtcMinKeep uint32 \`json:"xtc_min_keep"\`

	// XtcProbability controls XTC (eXtreme Token Culling) which randomly removes
	// tokens close to top probability. Must be > 0 to activate. Default is 0.0
	// (disabled).
	XtcProbability float32 \`json:"xtc_probability"\`

	// XtcThreshold is the probability threshold for XTC culling. Default is 0.1.
	XtcThreshold float32 \`json:"xtc_threshold"\`
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="type-rerankresponse">
              <h4>RerankResponse</h4>
              <pre className="code-block">
                <code>{`type RerankResponse struct {
	Object  string         \`json:"object"\`
	Created int64          \`json:"created"\`
	Model   string         \`json:"model"\`
	Data    []RerankResult \`json:"data"\`
	Usage   RerankUsage    \`json:"usage"\`
}`}</code>
              </pre>
              <p className="doc-description">RerankResponse represents the output for a reranking call.</p>
            </div>

            <div className="doc-section" id="type-rerankresult">
              <h4>RerankResult</h4>
              <pre className="code-block">
                <code>{`type RerankResult struct {
	Index          int     \`json:"index"\`
	RelevanceScore float32 \`json:"relevance_score"\`
	Document       string  \`json:"document,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">RerankResult represents a single document's reranking result.</p>
            </div>

            <div className="doc-section" id="type-rerankusage">
              <h4>RerankUsage</h4>
              <pre className="code-block">
                <code>{`type RerankUsage struct {
	PromptTokens int \`json:"prompt_tokens"\`
	TotalTokens  int \`json:"total_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">RerankUsage provides token usage information for reranking.</p>
            </div>

            <div className="doc-section" id="type-responsemessage">
              <h4>ResponseMessage</h4>
              <pre className="code-block">
                <code>{`type ResponseMessage struct {
	Role      string             \`json:"role,omitempty"\`
	Content   string             \`json:"content"\`
	Reasoning string             \`json:"reasoning_content,omitempty"\`
	ToolCalls []ResponseToolCall \`json:"tool_calls,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseMessage represents a single message in a response.</p>
            </div>

            <div className="doc-section" id="type-responsetoolcall">
              <h4>ResponseToolCall</h4>
              <pre className="code-block">
                <code>{`type ResponseToolCall struct {
	ID       string                   \`json:"id"\`
	Index    int                      \`json:"index"\`
	Type     string                   \`json:"type"\`
	Function ResponseToolCallFunction \`json:"function"\`
	Status   int                      \`json:"status,omitempty"\`
	Raw      string                   \`json:"raw,omitempty"\`
	Error    string                   \`json:"error,omitempty"\`
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="type-responsetoolcallfunction">
              <h4>ResponseToolCallFunction</h4>
              <pre className="code-block">
                <code>{`type ResponseToolCallFunction struct {
	Name      string            \`json:"name"\`
	Arguments ToolCallArguments \`json:"arguments"\`
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="type-ropescalingtype">
              <h4>RopeScalingType</h4>
              <pre className="code-block">
                <code>{`type RopeScalingType int32`}</code>
              </pre>
              <p className="doc-description">RopeScalingType controls RoPE (Rotary Position Embedding) scaling method. This enables extended context windows beyond the model's native training length. For example, Qwen3 models trained on 32k can support 131k with YaRN scaling.</p>
            </div>

            <div className="doc-section" id="type-splitmode">
              <h4>SplitMode</h4>
              <pre className="code-block">
                <code>{`type SplitMode int32`}</code>
              </pre>
              <p className="doc-description">SplitMode controls how the model is split across multiple GPUs. This is particularly important for Mixture of Experts (MoE) models.</p>
            </div>

            <div className="doc-section" id="type-streamingresponselogger">
              <h4>StreamingResponseLogger</h4>
              <pre className="code-block">
                <code>{`type StreamingResponseLogger struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">StreamingResponseLogger captures the final streaming response for logging. It must capture data before forwarding since the caller may mutate the response.</p>
            </div>

            <div className="doc-section" id="type-template">
              <h4>Template</h4>
              <pre className="code-block">
                <code>{`type Template struct {
	FileName string
	Script   string
}`}</code>
              </pre>
              <p className="doc-description">Template provides the template file name.</p>
            </div>

            <div className="doc-section" id="type-tokenizeresponse">
              <h4>TokenizeResponse</h4>
              <pre className="code-block">
                <code>{`type TokenizeResponse struct {
	Object  string \`json:"object"\`
	Created int64  \`json:"created"\`
	Model   string \`json:"model"\`
	Tokens  int    \`json:"tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">TokenizeResponse represents the output for a tokenize call.</p>
            </div>

            <div className="doc-section" id="type-toolcallarguments">
              <h4>ToolCallArguments</h4>
              <pre className="code-block">
                <code>{`type ToolCallArguments map[string]any`}</code>
              </pre>
              <p className="doc-description">ToolCallArguments represents tool call arguments that marshal to a JSON string per OpenAI API spec, but can unmarshal from either a string or object.</p>
            </div>

            <div className="doc-section" id="type-toplogprob">
              <h4>TopLogprob</h4>
              <pre className="code-block">
                <code>{`type TopLogprob struct {
	Token   string  \`json:"token"\`
	Logprob float32 \`json:"logprob"\`
	Bytes   []byte  \`json:"bytes,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">TopLogprob represents a single token with its log probability.</p>
            </div>

            <div className="doc-section" id="type-usage">
              <h4>Usage</h4>
              <pre className="code-block">
                <code>{`type Usage struct {
	PromptTokens        int     \`json:"prompt_tokens"\`
	ReasoningTokens     int     \`json:"reasoning_tokens"\`
	CompletionTokens    int     \`json:"completion_tokens"\`
	OutputTokens        int     \`json:"output_tokens"\`
	TotalTokens         int     \`json:"total_tokens"\`
	TokensPerSecond     float64 \`json:"tokens_per_second"\`
	TimeToFirstTokenMS  float64 \`json:"time_to_first_token_ms"\`
	DraftTokens         int     \`json:"draft_tokens,omitempty"\`
	DraftAcceptedTokens int     \`json:"draft_accepted_tokens,omitempty"\`
	DraftAcceptanceRate float64 \`json:"draft_acceptance_rate,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">Usage provides details usage information for the request.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-choice-finishreason">
              <h4>Choice.FinishReason</h4>
              <pre className="code-block">
                <code>func (c Choice) FinishReason() string</code>
              </pre>
              <p className="doc-description">FinishReason return the finish reason as an empty string if it is nil.</p>
            </div>

            <div className="doc-section" id="method-config-cachemintokens">
              <h4>Config.CacheMinTokens</h4>
              <pre className="code-block">
                <code>func (cfg Config) CacheMinTokens() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-cacheslottimeout">
              <h4>Config.CacheSlotTimeout</h4>
              <pre className="code-block">
                <code>func (cfg Config) CacheSlotTimeout() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-contextwindow">
              <h4>Config.ContextWindow</h4>
              <pre className="code-block">
                <code>func (cfg Config) ContextWindow() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-expertlayersongpu">
              <h4>Config.ExpertLayersOnGPU</h4>
              <pre className="code-block">
                <code>func (cfg Config) ExpertLayersOnGPU() int64</code>
              </pre>
              <p className="doc-description">ExpertLayersOnGPU translates the model's MoE configuration into the value the vram calculator expects so every prediction site (resman planner, post-load model logging, BUI display) reflects what llama.cpp will actually do at runtime. With no MoE override we mirror llama.cpp's default behavior: experts follow the layer they belong to, and full GPU offload puts every expert on the GPU. Without this resolution the calculator defaults experts to CPU even when the runtime puts them on GPU, producing the inverse of the placement that's actually loaded and silently under-accounting expert weight memory in the BUI VRAM column.</p>
            </div>

            <div className="doc-section" id="method-config-incrementalcache">
              <h4>Config.IncrementalCache</h4>
              <pre className="code-block">
                <code>func (cfg Config) IncrementalCache() bool</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-insecurelogging">
              <h4>Config.InsecureLogging</h4>
              <pre className="code-block">
                <code>func (cfg Config) InsecureLogging() bool</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-maingpu">
              <h4>Config.MainGPU</h4>
              <pre className="code-block">
                <code>func (cfg Config) MainGPU() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-nbatch">
              <h4>Config.NBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) NBatch() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-ngpulayers">
              <h4>Config.NGpuLayers</h4>
              <pre className="code-block">
                <code>func (cfg Config) NGpuLayers() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-nseqmax">
              <h4>Config.NSeqMax</h4>
              <pre className="code-block">
                <code>func (cfg Config) NSeqMax() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-nthreads">
              <h4>Config.NThreads</h4>
              <pre className="code-block">
                <code>func (cfg Config) NThreads() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-nthreadsbatch">
              <h4>Config.NThreadsBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) NThreadsBatch() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-nubatch">
              <h4>Config.NUBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) NUBatch() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-opoffloadminbatch">
              <h4>Config.OpOffloadMinBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) OpOffloadMinBatch() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-queuedepth">
              <h4>Config.QueueDepth</h4>
              <pre className="code-block">
                <code>func (cfg Config) QueueDepth() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-ropefreqbase">
              <h4>Config.RopeFreqBase</h4>
              <pre className="code-block">
                <code>func (cfg Config) RopeFreqBase() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-ropefreqscale">
              <h4>Config.RopeFreqScale</h4>
              <pre className="code-block">
                <code>func (cfg Config) RopeFreqScale() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-string">
              <h4>Config.String</h4>
              <pre className="code-block">
                <code>func (cfg Config) String() string</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-usedirectio">
              <h4>Config.UseDirectIO</h4>
              <pre className="code-block">
                <code>func (cfg Config) UseDirectIO() bool</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-yarnattnfactor">
              <h4>Config.YarnAttnFactor</h4>
              <pre className="code-block">
                <code>func (cfg Config) YarnAttnFactor() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-yarnbetafast">
              <h4>Config.YarnBetaFast</h4>
              <pre className="code-block">
                <code>func (cfg Config) YarnBetaFast() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-yarnbetaslow">
              <h4>Config.YarnBetaSlow</h4>
              <pre className="code-block">
                <code>func (cfg Config) YarnBetaSlow() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-yarnextfactor">
              <h4>Config.YarnExtFactor</h4>
              <pre className="code-block">
                <code>func (cfg Config) YarnExtFactor() float32</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-yarnorigctx">
              <h4>Config.YarnOrigCtx</h4>
              <pre className="code-block">
                <code>func (cfg Config) YarnOrigCtx() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-d-clone">
              <h4>D.Clone</h4>
              <pre className="code-block">
                <code>func (d D) Clone() D</code>
              </pre>
              <p className="doc-description">Clone creates a copy of the document. Top-level keys are copied into a new map. Values that are D or []D are cloned recursively so that nested message maps can be mutated independently across concurrent requests. Other value types (strings, numbers, etc.) are shared.</p>
            </div>

            <div className="doc-section" id="method-d-messages">
              <h4>D.Messages</h4>
              <pre className="code-block">
                <code>func (d D) Messages() string</code>
              </pre>
            </div>

            <div className="doc-section" id="method-d-shallowclone">
              <h4>D.ShallowClone</h4>
              <pre className="code-block">
                <code>func (d D) ShallowClone() D</code>
              </pre>
              <p className="doc-description">ShallowClone creates a copy of the top-level map and the messages slice. Individual message maps are shared with the original. Use this when downstream code treats message maps as read-only or performs its own copy-on-write when mutation is needed.</p>
            </div>

            <div className="doc-section" id="method-d-string">
              <h4>D.String</h4>
              <pre className="code-block">
                <code>func (d D) String() string</code>
              </pre>
              <p className="doc-description">String returns a string representation of the document containing only fields that are safe to log. This excludes sensitive fields like messages and input which may contain private user data.</p>
            </div>

            <div className="doc-section" id="method-draftmodelconfig-maingpu">
              <h4>DraftModelConfig.MainGPU</h4>
              <pre className="code-block">
                <code>func (d DraftModelConfig) MainGPU() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-draftmodelconfig-ngpulayers">
              <h4>DraftModelConfig.NGpuLayers</h4>
              <pre className="code-block">
                <code>func (d DraftModelConfig) NGpuLayers() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-flashattentiontype-marshaljson">
              <h4>FlashAttentionType.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (t FlashAttentionType) MarshalJSON() ([]byte, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-flashattentiontype-marshalyaml">
              <h4>FlashAttentionType.MarshalYAML</h4>
              <pre className="code-block">
                <code>func (t FlashAttentionType) MarshalYAML() (any, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-flashattentiontype-string">
              <h4>FlashAttentionType.String</h4>
              <pre className="code-block">
                <code>func (t FlashAttentionType) String() string</code>
              </pre>
            </div>

            <div className="doc-section" id="method-flashattentiontype-unmarshaljson">
              <h4>FlashAttentionType.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (t *FlashAttentionType) UnmarshalJSON(data []byte) error</code>
              </pre>
            </div>

            <div className="doc-section" id="method-flashattentiontype-unmarshalyaml">
              <h4>FlashAttentionType.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(any) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values.</p>
            </div>

            <div className="doc-section" id="method-ggmltype-marshaljson">
              <h4>GGMLType.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (t GGMLType) MarshalJSON() ([]byte, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ggmltype-marshalyaml">
              <h4>GGMLType.MarshalYAML</h4>
              <pre className="code-block">
                <code>func (t GGMLType) MarshalYAML() (any, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ggmltype-string">
              <h4>GGMLType.String</h4>
              <pre className="code-block">
                <code>func (t GGMLType) String() string</code>
              </pre>
              <p className="doc-description">String returns the string representation of a GGMLType.</p>
            </div>

            <div className="doc-section" id="method-ggmltype-toyzmatype">
              <h4>GGMLType.ToYZMAType</h4>
              <pre className="code-block">
                <code>func (t GGMLType) ToYZMAType() llama.GGMLType</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ggmltype-unmarshaljson">
              <h4>GGMLType.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (t *GGMLType) UnmarshalJSON(data []byte) error</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ggmltype-unmarshalyaml">
              <h4>GGMLType.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (t *GGMLType) UnmarshalYAML(unmarshal func(any) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".</p>
            </div>

            <div className="doc-section" id="method-moeconfig-keepexpertsongpufortopnlayers">
              <h4>MoEConfig.KeepExpertsOnGPUForTopNLayers</h4>
              <pre className="code-block">
                <code>func (m MoEConfig) KeepExpertsOnGPUForTopNLayers() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-model-chat">
              <h4>Model.Chat</h4>
              <pre className="code-block">
                <code>func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error)</code>
              </pre>
              <p className="doc-description">Chat performs a chat request and returns the final response. All requests (including vision/audio) use batch processing and can run concurrently based on the NSeqMax config value, which controls parallel sequence processing.</p>
            </div>

            <div className="doc-section" id="method-model-chatstreaming">
              <h4>Model.ChatStreaming</h4>
              <pre className="code-block">
                <code>func (m *Model) ChatStreaming(ctx context.Context, d D) &lt;-chan ChatResponse</code>
              </pre>
              <p className="doc-description">ChatStreaming performs a chat request and streams the response. All requests (including vision/audio) use batch processing and can run concurrently based on the NSeqMax config value, which controls parallel sequence processing.</p>
            </div>

            <div className="doc-section" id="method-model-config">
              <h4>Model.Config</h4>
              <pre className="code-block">
                <code>func (m *Model) Config() Config</code>
              </pre>
            </div>

            <div className="doc-section" id="method-model-embeddings">
              <h4>Model.Embeddings</h4>
              <pre className="code-block">
                <code>func (m *Model) Embeddings(ctx context.Context, d D) (EmbedReponse, error)</code>
              </pre>
              <p className="doc-description">Embeddings performs embedding for one or more inputs. Supported options in d: - input ([]string): the texts to embed (required) - truncate (bool): if true, truncate inputs to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models) When NSeqMax &gt; 1, multiple concurrent requests can be processed in parallel, each using one context from the internal pool.</p>
            </div>

            <div className="doc-section" id="method-model-modelinfo">
              <h4>Model.ModelInfo</h4>
              <pre className="code-block">
                <code>func (m *Model) ModelInfo() ModelInfo</code>
              </pre>
            </div>

            <div className="doc-section" id="method-model-rerank">
              <h4>Model.Rerank</h4>
              <pre className="code-block">
                <code>func (m *Model) Rerank(ctx context.Context, d D) (RerankResponse, error)</code>
              </pre>
              <p className="doc-description">Rerank performs reranking for a query against multiple documents. It scores each document's relevance to the query and returns results sorted by relevance score (highest first). Supported options in d: - query (string): the query to rank documents against (required) - documents ([]string): the documents to rank (required) - top_n (int): return only the top N results (optional, default: all) - return_documents (bool): include document text in results (default: false) When NSeqMax &gt; 1, multiple concurrent requests can be processed in parallel, each using one context from the internal pool.</p>
            </div>

            <div className="doc-section" id="method-model-tokenize">
              <h4>Model.Tokenize</h4>
              <pre className="code-block">
                <code>func (m *Model) Tokenize(ctx context.Context, d D) (TokenizeResponse, error)</code>
              </pre>
              <p className="doc-description">Tokenize returns the token count for a text input. Supported options in d: - input (string): the text to tokenize (required) - apply_template (bool): if true, wrap input as a user message and apply the model's chat template before tokenizing (default: false) - add_generation_prompt (bool): when apply_template is true, controls whether the assistant role prefix is appended to the prompt (default: true) When apply_template is true, the returned count includes all template overhead (role markers, separators, generation prompt). This reflects the actual number of tokens that would be fed to the model.</p>
            </div>

            <div className="doc-section" id="method-model-unload">
              <h4>Model.Unload</h4>
              <pre className="code-block">
                <code>func (m *Model) Unload(ctx context.Context) error</code>
              </pre>
            </div>

            <div className="doc-section" id="method-modelinfo-string">
              <h4>ModelInfo.String</h4>
              <pre className="code-block">
                <code>func (mi ModelInfo) String() string</code>
              </pre>
            </div>

            <div className="doc-section" id="method-modeltype-string">
              <h4>ModelType.String</h4>
              <pre className="code-block">
                <code>func (mt ModelType) String() string</code>
              </pre>
              <p className="doc-description">String returns a human-readable name for the model type.</p>
            </div>

            <div className="doc-section" id="method-params-string">
              <h4>Params.String</h4>
              <pre className="code-block">
                <code>func (p Params) String() string</code>
              </pre>
              <p className="doc-description">String returns a string representation of the Params containing only non-zero values in the format key[value]\nkey[value]\n ...</p>
            </div>

            <div className="doc-section" id="method-ropescalingtype-marshaljson">
              <h4>RopeScalingType.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (r RopeScalingType) MarshalJSON() ([]byte, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ropescalingtype-marshalyaml">
              <h4>RopeScalingType.MarshalYAML</h4>
              <pre className="code-block">
                <code>func (r RopeScalingType) MarshalYAML() (any, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ropescalingtype-string">
              <h4>RopeScalingType.String</h4>
              <pre className="code-block">
                <code>func (r RopeScalingType) String() string</code>
              </pre>
              <p className="doc-description">String returns the string representation of a RopeScalingType.</p>
            </div>

            <div className="doc-section" id="method-ropescalingtype-toyzmatype">
              <h4>RopeScalingType.ToYZMAType</h4>
              <pre className="code-block">
                <code>func (r RopeScalingType) ToYZMAType() llama.RopeScalingType</code>
              </pre>
              <p className="doc-description">ToYZMAType converts to the yzma/llama.cpp RopeScalingType.</p>
            </div>

            <div className="doc-section" id="method-ropescalingtype-unmarshaljson">
              <h4>RopeScalingType.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (r *RopeScalingType) UnmarshalJSON(data []byte) error</code>
              </pre>
            </div>

            <div className="doc-section" id="method-ropescalingtype-unmarshalyaml">
              <h4>RopeScalingType.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (r *RopeScalingType) UnmarshalYAML(unmarshal func(any) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values.</p>
            </div>

            <div className="doc-section" id="method-splitmode-marshaljson">
              <h4>SplitMode.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (s SplitMode) MarshalJSON() ([]byte, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-splitmode-marshalyaml">
              <h4>SplitMode.MarshalYAML</h4>
              <pre className="code-block">
                <code>func (s SplitMode) MarshalYAML() (any, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-splitmode-string">
              <h4>SplitMode.String</h4>
              <pre className="code-block">
                <code>func (s SplitMode) String() string</code>
              </pre>
              <p className="doc-description">String returns the string representation of a SplitMode.</p>
            </div>

            <div className="doc-section" id="method-splitmode-toyzmatype">
              <h4>SplitMode.ToYZMAType</h4>
              <pre className="code-block">
                <code>func (s SplitMode) ToYZMAType() llama.SplitMode</code>
              </pre>
              <p className="doc-description">ToYZMAType converts to the yzma/llama.cpp SplitMode type.</p>
            </div>

            <div className="doc-section" id="method-splitmode-unmarshaljson">
              <h4>SplitMode.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (s *SplitMode) UnmarshalJSON(data []byte) error</code>
              </pre>
            </div>

            <div className="doc-section" id="method-splitmode-unmarshalyaml">
              <h4>SplitMode.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (s *SplitMode) UnmarshalYAML(unmarshal func(any) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values.</p>
            </div>

            <div className="doc-section" id="method-streamingresponselogger-capture">
              <h4>StreamingResponseLogger.Capture</h4>
              <pre className="code-block">
                <code>func (l *StreamingResponseLogger) Capture(resp ChatResponse)</code>
              </pre>
              <p className="doc-description">Capture captures data from a streaming response. Call this for each response before forwarding it. It only captures from the final response (when FinishReason is set).</p>
            </div>

            <div className="doc-section" id="method-streamingresponselogger-string">
              <h4>StreamingResponseLogger.String</h4>
              <pre className="code-block">
                <code>func (l *StreamingResponseLogger) String() string</code>
              </pre>
              <p className="doc-description">String returns a formatted string for logging.</p>
            </div>

            <div className="doc-section" id="method-toolcallarguments-marshaljson">
              <h4>ToolCallArguments.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (a ToolCallArguments) MarshalJSON() ([]byte, error)</code>
              </pre>
            </div>

            <div className="doc-section" id="method-toolcallarguments-unmarshaljson">
              <h4>ToolCallArguments.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (a *ToolCallArguments) UnmarshalJSON(data []byte) error</code>
              </pre>
            </div>
          </div>

          <div className="card" id="constants">
            <h3>Constants</h3>

            <div className="doc-section" id="const-numadisabled">
              <h4>NUMADisabled</h4>
              <pre className="code-block">
                <code>{`const (
	NUMADisabled   = ""
	NUMADistribute = "distribute"
	NUMAIsolate    = "isolate"
	NUMANumactl    = "numactl"
	NUMAMirror     = "mirror"
)`}</code>
              </pre>
            </div>

            <div className="doc-section" id="const-objectchatunknown">
              <h4>ObjectChatUnknown</h4>
              <pre className="code-block">
                <code>{`const (
	ObjectChatUnknown   = "chat.unknown"
	ObjectChatText      = "chat.completion.chunk"
	ObjectChatTextFinal = "chat.completion"
	ObjectChatMedia     = "chat.media"
)`}</code>
              </pre>
              <p className="doc-description">Objects represent the different types of data that is being processed.</p>
            </div>

            <div className="doc-section" id="const-roleuser">
              <h4>RoleUser</h4>
              <pre className="code-block">
                <code>{`const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"
)`}</code>
              </pre>
              <p className="doc-description">Roles represent the different roles that can be used in a chat.</p>
            </div>

            <div className="doc-section" id="const-finishreasonstop">
              <h4>FinishReasonStop</h4>
              <pre className="code-block">
                <code>{`const (
	FinishReasonStop  = "stop"
	FinishReasonTool  = "tool_calls"
	FinishReasonError = "error"
)`}</code>
              </pre>
              <p className="doc-description">FinishReasons represent the different reasons a response can be finished.</p>
            </div>

            <div className="doc-section" id="const-defadaptivepdecay">
              <h4>DefAdaptivePDecay</h4>
              <pre className="code-block">
                <code>{`const (
	// DefAdaptivePDecay controls how quickly the Adaptive-P sampler adjusts.
	DefAdaptivePDecay = 0.0

	// DefAdaptivePTarget is the target probability threshold for Adaptive-P
	// sampling. When > 0, enables adaptive sampling that dynamically adjusts
	// based on the probability distribution to prevent predictable patterns.
	DefAdaptivePTarget = 0.0

	// DefDryAllowedLen is the minimum n-gram length before DRY applies.
	DefDryAllowedLen = 2

	// DefDryBase is the base for exponential penalty growth in DRY.
	DefDryBase = 1.75

	// DefDryMultiplier controls the DRY (Don't Repeat Yourself) sampler which penalizes
	// n-gram pattern repetition. 0.8 - Light repetition penalty,
	// 1.0–1.5 - Moderate (typical starting point), 2.0–3.0 - Aggressive.
	// Default is 0.0 (disabled) to match Ollama and maximize tool calling stability.
	DefDryMultiplier = 0.0

	// DefDryPenaltyLast limits how many recent tokens DRY considers.
	DefDryPenaltyLast = 0.0

	// DefEnableThinking determines if the model should think or not. It is used for
	// most non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False.
	DefEnableThinking = ThinkingEnabled

	// DefFrequencyPenalty penalizes tokens proportionally to how often they have
	// appeared in the output. Higher values more strongly discourage frequent
	// repetition. Default is 0.0 (disabled).
	DefFrequencyPenalty float32 = 0.0

	// DefIncludeUsage determines whether to include token usage information in
	// streaming responses.
	DefIncludeUsage = true

	// DefLogprobs determines whether to return log probabilities of output tokens.
	// When enabled, the response includes probability data for each generated token.
	DefLogprobs = false

	// DefMaxTokens exists for backward compatibility. When max_tokens is
	// not specified in a request, adjustParams defaults to the model's
	// context window size.
	DefMaxTokens = 4096

	// DefMaxTopLogprobs defines the number of maximum logprobs to use.
	DefMaxTopLogprobs = 5

	// DefMinP is a dynamic sampling threshold that helps balance the coherence
	// (quality) and diversity (creativity) of the generated text.
	DefMinP = 0.0

	// DefPresencePenalty applies a flat penalty to any token that has already
	// appeared in the output, regardless of frequency. Higher values encourage
	// the model to introduce new topics. Default is 0.0 (disabled).
	DefPresencePenalty float32 = 0.0

	// DefReasoningEffort is a string that specifies the level of reasoning effort to
	// use for GPT models.
	DefReasoningEffort = ReasoningEffortMedium

	// DefRepeatLastN specifies how many recent tokens to consider when applying the
	// repetition penalty. A larger value considers more context but may be slower.
	DefRepeatLastN = 64

	// DefRepeatPenalty applies a penalty to tokens that have already appeared in the
	// output, reducing repetitive text. A value of 1.0 means no penalty. Values
	// above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is strong).
	// Default is 1.0 (disabled) because even mild penalties suppress structural
	// JSON tokens like { in tool call formats (e.g., Gemma's call:func{{...}}),
	// causing the model to substitute [ for { and producing invalid arguments.
	DefRepeatPenalty = 1.0

	// DefReturnPrompt determines whether to include the prompt in the final response.
	// When set to true, the prompt will be included.
	DefReturnPrompt = false

	// DefTemp controls the randomness of the output. It rescales the probability
	// distribution of possible next tokens.
	DefTemp = 0.8

	// DefTopK limits the pool of possible next tokens to the K number of most
	// probable tokens. Default is 40 to match Ollama and cut off low-probability
	// tokens that can break structured output like tool calls.
	DefTopK int32 = 40

	// DefTopLogprobs specifies how many of the most likely tokens to return at each
	// position, along with their log probabilities. Must be between 0 and 5.
	// Setting this to a value > 0 implicitly enables logprobs.
	DefTopLogprobs = 0

	// DefTopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the minimum
	// number of most probable tokens required to reach the cumulative probability P.
	DefTopP = 0.9

	// DefXtcMinKeep is the minimum tokens to keep after XTC culling.
	DefXtcMinKeep = 1

	// DefXtcProbability controls XTC (eXtreme Token Culling) which randomly removes
	// tokens close to top probability. Must be > 0 to activate.
	DefXtcProbability = 0.0

	// DefXtcThreshold is the probability threshold for XTC culling.
	DefXtcThreshold = 0.1
)`}</code>
              </pre>
            </div>

            <div className="doc-section" id="const-thinkingenabled">
              <h4>ThinkingEnabled</h4>
              <pre className="code-block">
                <code>{`const (
	// The model will perform thinking. This is the default setting.
	ThinkingEnabled = "true"

	// The model will not perform thinking.
	ThinkingDisabled = "false"
)`}</code>
              </pre>
            </div>

            <div className="doc-section" id="const-reasoningeffortnone">
              <h4>ReasoningEffortNone</h4>
              <pre className="code-block">
                <code>{`const (
	// The model does not perform reasoning This setting is fastest and lowest
	// cost, ideal for latency-sensitive tasks that do not require complex logic,
	// such as simple translation or data reformatting.
	ReasoningEffortNone = "none"

	// GPT: A very low amount of internal reasoning, optimized for throughput
	// and speed.
	ReasoningEffortMinimal = "minimal"

	// GPT: Light reasoning that favors speed and lower token usage, suitable
	// for triage or short answers.
	ReasoningEffortLow = "low"

	// GPT: The default setting, providing a balance between speed and reasoning
	// accuracy. This is a good general-purpose choice for most tasks like
	// content drafting or standard Q&A.
	ReasoningEffortMedium = "medium"

	// GPT: Extensive reasoning for complex, multi-step problems. This setting
	// leads to the most thorough and accurate analysis but increases latency
	// and cost due to a larger number of internal reasoning tokens used.
	ReasoningEffortHigh = "high"
)`}</code>
              </pre>
            </div>

            <div className="doc-section" id="const-expertsallongpu">
              <h4>ExpertsAllOnGPU</h4>
              <pre className="code-block">
                <code>{`const ExpertsAllOnGPU = int64(math.MaxInt32)`}</code>
              </pre>
              <p className="doc-description">ExpertsAllOnGPU is the sentinel value used for vram.Config.ExpertLayersOnGPU to request that every routed-expert layer be charged against GPU VRAM. The vram package treats any value greater than or equal to the model's block count as "all layers on GPU", so a large constant works regardless of model depth and avoids a metadata round-trip just to discover it.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-addparams">AddParams</a></li>
                <li><a href="#func-checkmodel">CheckModel</a></li>
                <li><a href="#func-inityzmaworkarounds">InitYzmaWorkarounds</a></li>
                <li><a href="#func-newgrammarsampler">NewGrammarSampler</a></li>
                <li><a href="#func-parseggmltype">ParseGGMLType</a></li>
                <li><a href="#func-newmodel">NewModel</a></li>
                <li><a href="#func-detectmodeltypefromfiles">DetectModelTypeFromFiles</a></li>
                <li><a href="#func-parseropescalingtype">ParseRopeScalingType</a></li>
                <li><a href="#func-parsesplitmode">ParseSplitMode</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-chatresponse">ChatResponse</a></li>
                <li><a href="#type-choice">Choice</a></li>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-contentlogprob">ContentLogprob</a></li>
                <li><a href="#type-d">D</a></li>
                <li><a href="#type-draftmodelconfig">DraftModelConfig</a></li>
                <li><a href="#type-embeddata">EmbedData</a></li>
                <li><a href="#type-embedreponse">EmbedReponse</a></li>
                <li><a href="#type-embedusage">EmbedUsage</a></li>
                <li><a href="#type-flashattentiontype">FlashAttentionType</a></li>
                <li><a href="#type-ggmltype">GGMLType</a></li>
                <li><a href="#type-logger">Logger</a></li>
                <li><a href="#type-logprobs">Logprobs</a></li>
                <li><a href="#type-mediatype">MediaType</a></li>
                <li><a href="#type-moeconfig">MoEConfig</a></li>
                <li><a href="#type-moemode">MoEMode</a></li>
                <li><a href="#type-model">Model</a></li>
                <li><a href="#type-modelinfo">ModelInfo</a></li>
                <li><a href="#type-modeltype">ModelType</a></li>
                <li><a href="#type-option">Option</a></li>
                <li><a href="#type-params">Params</a></li>
                <li><a href="#type-rerankresponse">RerankResponse</a></li>
                <li><a href="#type-rerankresult">RerankResult</a></li>
                <li><a href="#type-rerankusage">RerankUsage</a></li>
                <li><a href="#type-responsemessage">ResponseMessage</a></li>
                <li><a href="#type-responsetoolcall">ResponseToolCall</a></li>
                <li><a href="#type-responsetoolcallfunction">ResponseToolCallFunction</a></li>
                <li><a href="#type-ropescalingtype">RopeScalingType</a></li>
                <li><a href="#type-splitmode">SplitMode</a></li>
                <li><a href="#type-streamingresponselogger">StreamingResponseLogger</a></li>
                <li><a href="#type-template">Template</a></li>
                <li><a href="#type-tokenizeresponse">TokenizeResponse</a></li>
                <li><a href="#type-toolcallarguments">ToolCallArguments</a></li>
                <li><a href="#type-toplogprob">TopLogprob</a></li>
                <li><a href="#type-usage">Usage</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-choice-finishreason">Choice.FinishReason</a></li>
                <li><a href="#method-config-cachemintokens">Config.CacheMinTokens</a></li>
                <li><a href="#method-config-cacheslottimeout">Config.CacheSlotTimeout</a></li>
                <li><a href="#method-config-contextwindow">Config.ContextWindow</a></li>
                <li><a href="#method-config-expertlayersongpu">Config.ExpertLayersOnGPU</a></li>
                <li><a href="#method-config-incrementalcache">Config.IncrementalCache</a></li>
                <li><a href="#method-config-insecurelogging">Config.InsecureLogging</a></li>
                <li><a href="#method-config-maingpu">Config.MainGPU</a></li>
                <li><a href="#method-config-nbatch">Config.NBatch</a></li>
                <li><a href="#method-config-ngpulayers">Config.NGpuLayers</a></li>
                <li><a href="#method-config-nseqmax">Config.NSeqMax</a></li>
                <li><a href="#method-config-nthreads">Config.NThreads</a></li>
                <li><a href="#method-config-nthreadsbatch">Config.NThreadsBatch</a></li>
                <li><a href="#method-config-nubatch">Config.NUBatch</a></li>
                <li><a href="#method-config-opoffloadminbatch">Config.OpOffloadMinBatch</a></li>
                <li><a href="#method-config-queuedepth">Config.QueueDepth</a></li>
                <li><a href="#method-config-ropefreqbase">Config.RopeFreqBase</a></li>
                <li><a href="#method-config-ropefreqscale">Config.RopeFreqScale</a></li>
                <li><a href="#method-config-string">Config.String</a></li>
                <li><a href="#method-config-usedirectio">Config.UseDirectIO</a></li>
                <li><a href="#method-config-yarnattnfactor">Config.YarnAttnFactor</a></li>
                <li><a href="#method-config-yarnbetafast">Config.YarnBetaFast</a></li>
                <li><a href="#method-config-yarnbetaslow">Config.YarnBetaSlow</a></li>
                <li><a href="#method-config-yarnextfactor">Config.YarnExtFactor</a></li>
                <li><a href="#method-config-yarnorigctx">Config.YarnOrigCtx</a></li>
                <li><a href="#method-d-clone">D.Clone</a></li>
                <li><a href="#method-d-messages">D.Messages</a></li>
                <li><a href="#method-d-shallowclone">D.ShallowClone</a></li>
                <li><a href="#method-d-string">D.String</a></li>
                <li><a href="#method-draftmodelconfig-maingpu">DraftModelConfig.MainGPU</a></li>
                <li><a href="#method-draftmodelconfig-ngpulayers">DraftModelConfig.NGpuLayers</a></li>
                <li><a href="#method-flashattentiontype-marshaljson">FlashAttentionType.MarshalJSON</a></li>
                <li><a href="#method-flashattentiontype-marshalyaml">FlashAttentionType.MarshalYAML</a></li>
                <li><a href="#method-flashattentiontype-string">FlashAttentionType.String</a></li>
                <li><a href="#method-flashattentiontype-unmarshaljson">FlashAttentionType.UnmarshalJSON</a></li>
                <li><a href="#method-flashattentiontype-unmarshalyaml">FlashAttentionType.UnmarshalYAML</a></li>
                <li><a href="#method-ggmltype-marshaljson">GGMLType.MarshalJSON</a></li>
                <li><a href="#method-ggmltype-marshalyaml">GGMLType.MarshalYAML</a></li>
                <li><a href="#method-ggmltype-string">GGMLType.String</a></li>
                <li><a href="#method-ggmltype-toyzmatype">GGMLType.ToYZMAType</a></li>
                <li><a href="#method-ggmltype-unmarshaljson">GGMLType.UnmarshalJSON</a></li>
                <li><a href="#method-ggmltype-unmarshalyaml">GGMLType.UnmarshalYAML</a></li>
                <li><a href="#method-moeconfig-keepexpertsongpufortopnlayers">MoEConfig.KeepExpertsOnGPUForTopNLayers</a></li>
                <li><a href="#method-model-chat">Model.Chat</a></li>
                <li><a href="#method-model-chatstreaming">Model.ChatStreaming</a></li>
                <li><a href="#method-model-config">Model.Config</a></li>
                <li><a href="#method-model-embeddings">Model.Embeddings</a></li>
                <li><a href="#method-model-modelinfo">Model.ModelInfo</a></li>
                <li><a href="#method-model-rerank">Model.Rerank</a></li>
                <li><a href="#method-model-tokenize">Model.Tokenize</a></li>
                <li><a href="#method-model-unload">Model.Unload</a></li>
                <li><a href="#method-modelinfo-string">ModelInfo.String</a></li>
                <li><a href="#method-modeltype-string">ModelType.String</a></li>
                <li><a href="#method-params-string">Params.String</a></li>
                <li><a href="#method-ropescalingtype-marshaljson">RopeScalingType.MarshalJSON</a></li>
                <li><a href="#method-ropescalingtype-marshalyaml">RopeScalingType.MarshalYAML</a></li>
                <li><a href="#method-ropescalingtype-string">RopeScalingType.String</a></li>
                <li><a href="#method-ropescalingtype-toyzmatype">RopeScalingType.ToYZMAType</a></li>
                <li><a href="#method-ropescalingtype-unmarshaljson">RopeScalingType.UnmarshalJSON</a></li>
                <li><a href="#method-ropescalingtype-unmarshalyaml">RopeScalingType.UnmarshalYAML</a></li>
                <li><a href="#method-splitmode-marshaljson">SplitMode.MarshalJSON</a></li>
                <li><a href="#method-splitmode-marshalyaml">SplitMode.MarshalYAML</a></li>
                <li><a href="#method-splitmode-string">SplitMode.String</a></li>
                <li><a href="#method-splitmode-toyzmatype">SplitMode.ToYZMAType</a></li>
                <li><a href="#method-splitmode-unmarshaljson">SplitMode.UnmarshalJSON</a></li>
                <li><a href="#method-splitmode-unmarshalyaml">SplitMode.UnmarshalYAML</a></li>
                <li><a href="#method-streamingresponselogger-capture">StreamingResponseLogger.Capture</a></li>
                <li><a href="#method-streamingresponselogger-string">StreamingResponseLogger.String</a></li>
                <li><a href="#method-toolcallarguments-marshaljson">ToolCallArguments.MarshalJSON</a></li>
                <li><a href="#method-toolcallarguments-unmarshaljson">ToolCallArguments.UnmarshalJSON</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#constants" className="doc-index-header">Constants</a>
              <ul>
                <li><a href="#const-numadisabled">NUMADisabled</a></li>
                <li><a href="#const-objectchatunknown">ObjectChatUnknown</a></li>
                <li><a href="#const-roleuser">RoleUser</a></li>
                <li><a href="#const-finishreasonstop">FinishReasonStop</a></li>
                <li><a href="#const-defadaptivepdecay">DefAdaptivePDecay</a></li>
                <li><a href="#const-thinkingenabled">ThinkingEnabled</a></li>
                <li><a href="#const-reasoningeffortnone">ReasoningEffortNone</a></li>
                <li><a href="#const-expertsallongpu">ExpertsAllOnGPU</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
