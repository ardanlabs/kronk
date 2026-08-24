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

            <div className="doc-section" id="func-detectmodeltypefromfiles">
              <h4>DetectModelTypeFromFiles</h4>
              <pre className="code-block">
                <code>func DetectModelTypeFromFiles(modelFiles []string) (ModelType, string, error)</code>
              </pre>
              <p className="doc-description">DetectModelTypeFromFiles loads a model from the given GGUF files, determines the architecture type, and immediately frees the model. It returns the ModelType, the raw general.architecture string from the GGUF metadata, and any error encountered during loading.</p>
            </div>

            <div className="doc-section" id="func-getembeddingsprenorm">
              <h4>GetEmbeddingsPreNorm</h4>
              <pre className="code-block">
                <code>func GetEmbeddingsPreNorm(ctx llama.Context, nRows, nEmbd int) []float32</code>
              </pre>
              <p className="doc-description">GetEmbeddingsPreNorm returns the dense pre-norm hidden-state buffer produced by the most recent llama_decode on ctx. nRows is the number of rows the caller expects (typically batch.NTokens for an unmasked context); nEmbd is the model's embedding width (llama.ModelNEmbd). Returns nil when the binding isn't loaded, the context is zero, or the underlying C call returned NULL (no pre-norm buffer available — usually means SetEmbeddingsPreNorm wasn't enabled before the decode). The returned slice aliases C-owned memory; the caller MUST NOT retain it past the next decode/synchronize call. Copy out rows that need to survive.</p>
            </div>

            <div className="doc-section" id="func-getembeddingsprenormith">
              <h4>GetEmbeddingsPreNormIth</h4>
              <pre className="code-block">
                <code>func GetEmbeddingsPreNormIth(ctx llama.Context, i int32, nEmbd int) []float32</code>
              </pre>
              <p className="doc-description">GetEmbeddingsPreNormIth returns the pre-norm hidden-state row for the ith output of the most recent llama_decode on ctx. nEmbd is the model's embedding width. On a masked context (ctx_dft for MTP) i indexes through the output_ids table, so it must correspond to a batch position whose logits flag was set. On an unmasked context (ctx_tgt) i is the raw batch position. Returns nil when the binding isn't loaded, the context is zero, or the row isn't available. The returned slice aliases C-owned memory; don't retain past the next decode/synchronize.</p>
            </div>

            <div className="doc-section" id="func-inityzmaworkarounds">
              <h4>InitYzmaWorkarounds</h4>
              <pre className="code-block">
                <code>func InitYzmaWorkarounds(libPath string) error</code>
              </pre>
              <p className="doc-description">InitYzmaWorkarounds loads the llama library and preps our extra FFI functions that yzma upstream doesn't bind yet. Safe to call multiple times; only the first call does any work. Pre-norm bindings are BEST-EFFORT: if the loaded llama library doesn't export them (older build, e.g. b9222), the corresponding ffi.Fun stays zero-valued and MTPAvailable() returns false. Init never fails on a missing pre-norm symbol so kronk still boots and can serve non-MTP models.</p>
            </div>

            <div className="doc-section" id="func-mtpavailable">
              <h4>MTPAvailable</h4>
              <pre className="code-block">
                <code>func MTPAvailable() bool</code>
              </pre>
              <p className="doc-description">MTPAvailable reports whether the loaded llama library exports the three pre-norm hidden-state symbols required for MTP speculative decoding. Older llama.cpp builds (pre src/llama-ext.h pre-norm API) won't have them; the MTP auto-detect path checks this and skips silently when false, so kronk still starts up — it just runs without MTP speculation.</p>
            </div>

            <div className="doc-section" id="func-newmodel">
              <h4>NewModel</h4>
              <pre className="code-block">
                <code>func NewModel(ctx context.Context, cfg Config) (*Model, error)</code>
              </pre>
              <p className="doc-description">NewModel loads a model from the GGUF files specified in cfg and returns a *Model ready to serve requests. It validates the configuration, builds llama.cpp model parameters, applies NUMA settings, performs the actual GGUF load (serialized via a process-wide mutex to guard the GGML_OP_OFFLOAD_MIN_BATCH env var), computes VRAM/KV diagnostics, retrieves the chat template, and initializes the per-model runtime — either the sequence-batch runtime or context-pool fallback for embed/rerank models, or a batch engine plus parser plugin and optional draft model for generation models. The returned *Model owns the underlying llama.Model, llama.Context, KV memory, batch engine, and (when configured) draft model; release them via Model.Unload when finished.</p>
            </div>

            <div className="doc-section" id="func-parseggmltype">
              <h4>ParseGGMLType</h4>
              <pre className="code-block">
                <code>func ParseGGMLType(s string) (GGMLType, error)</code>
              </pre>
              <p className="doc-description">ParseGGMLType parses a string into a GGMLType. Supported values: "f32", "f16", "q4_0", "q4_1", "q5_0", "q5_1", "q8_0", "bf16", "auto".</p>
            </div>

            <div className="doc-section" id="func-parseloadmode">
              <h4>ParseLoadMode</h4>
              <pre className="code-block">
                <code>func ParseLoadMode(s string) (LoadMode, error)</code>
              </pre>
              <p className="doc-description">ParseLoadMode parses a string into a LoadMode.</p>
            </div>

            <div className="doc-section" id="func-parsemoemode">
              <h4>ParseMoEMode</h4>
              <pre className="code-block">
                <code>func ParseMoEMode(value string) (MoEMode, error)</code>
              </pre>
              <p className="doc-description">ParseMoEMode parses value and returns the corresponding MoEMode when it exists.</p>
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
              <p className="doc-description">ParseSplitMode parses a string into a SplitMode. Supported values are "none", "layer", and "row". The legacy aliases "tensor", "tensor-parallel", and "expert-parallel" map to row mode.</p>
            </div>

            <div className="doc-section" id="func-recurrentstatecopies">
              <h4>RecurrentStateCopies</h4>
              <pre className="code-block">
                <code>func RecurrentStateCopies(cfg Config, embeddedMTP bool) int64</code>
              </pre>
              <p className="doc-description">RecurrentStateCopies returns the number of current and rollback recurrent state planes allocated for the configured speculative-decoding mode. embeddedMTP selects the auto-detected MTP path; false selects an explicit separate drafter or companion MTP file.</p>
            </div>

            <div className="doc-section" id="func-registerparser">
              <h4>RegisterParser</h4>
              <pre className="code-block">
                <code>func RegisterParser(f ParserFactory)</code>
              </pre>
              <p className="doc-description">RegisterParser appends a parser factory to the registry. Call once per parser at server bootstrap, before any models are loaded. Order matters: the catch-all parser (fallback) must be registered last so the more specific parsers get first chance to claim.</p>
            </div>

            <div className="doc-section" id="func-setembeddingsprenorm">
              <h4>SetEmbeddingsPreNorm</h4>
              <pre className="code-block">
                <code>func SetEmbeddingsPreNorm(ctx llama.Context, value, masked bool)</code>
              </pre>
              <p className="doc-description">SetEmbeddingsPreNorm enables (or disables) pre-norm hidden-state extraction on the given context. - value == true: the next llama_decode will produce a pre-norm embedding buffer accessible via GetEmbeddingsPreNorm / GetEmbeddingsPreNormIth. - masked == false: rows are stored densely, indexed by raw batch position. Used on the target context (caller wants every row). - masked == true: rows are stored only for batch positions whose logits flag is non-zero, indexed via the output_ids table. Used on the MTP draft context (caller only needs the output rows). Mirrors llama_set_embeddings_pre_norm in src/llama-ext.h.</p>
            </div>

            <div className="doc-section" id="func-validatechatrequest">
              <h4>ValidateChatRequest</h4>
              <pre className="code-block">
                <code>func ValidateChatRequest(d D) error</code>
              </pre>
              <p className="doc-description">ValidateChatRequest validates the fields in a chat request document.</p>
            </div>

            <div className="doc-section" id="func-validatemessages">
              <h4>ValidateMessages</h4>
              <pre className="code-block">
                <code>func ValidateMessages(d D) error</code>
              </pre>
              <p className="doc-description">ValidateMessages validates the messages field of a chat document.</p>
            </div>

            <div className="doc-section" id="func-verifyartifact">
              <h4>VerifyArtifact</h4>
              <pre className="code-block">
                <code>func VerifyArtifact(modelFile string, digest ArtifactDigest, previous *ArtifactVerification, checkSHA bool) (ArtifactVerification, bool, error)</code>
              </pre>
              <p className="doc-description">VerifyArtifact checks modelFile against parsed integrity values. It reads the artifact itself but performs no sidecar I/O. The returned bool reports whether a new verification was produced and should be persisted by the caller.</p>
            </div>

            <div className="doc-section" id="func-newgrammarsampler">
              <h4>NewGrammarSampler</h4>
              <pre className="code-block">
                <code>func NewGrammarSampler(vocab llama.Vocab, grammar string) *grammarSampler</code>
              </pre>
              <p className="doc-description">NewGrammarSampler creates a grammar sampler that will be managed separately from the main sampler chain.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-adapterconfig">
              <h4>AdapterConfig</h4>
              <pre className="code-block">
                <code>{`type AdapterConfig struct {
	Path  string
	Scale float32
}`}</code>
              </pre>
              <p className="doc-description">AdapterConfig configures a local llama.cpp-compatible LoRA adapter GGUF. Scale is fixed for the lifetime of the loaded model.</p>
            </div>

            <div className="doc-section" id="type-artifactdigest">
              <h4>ArtifactDigest</h4>
              <pre className="code-block">
                <code>{`type ArtifactDigest struct {
	SHA256 string
	Size   int64
}`}</code>
              </pre>
              <p className="doc-description">ArtifactDigest provides the expected SHA256 and size for an artifact.</p>
            </div>

            <div className="doc-section" id="type-artifactintegrity">
              <h4>ArtifactIntegrity</h4>
              <pre className="code-block">
                <code>{`type ArtifactIntegrity struct {
	Digest       ArtifactDigest
	Verification *ArtifactVerification
}`}</code>
              </pre>
              <p className="doc-description">ArtifactIntegrity provides the parsed digest and optional prior verification for an artifact path.</p>
            </div>

            <div className="doc-section" id="type-artifactverification">
              <h4>ArtifactVerification</h4>
              <pre className="code-block">
                <code>{`type ArtifactVerification struct {
	SHA256       string
	Size         int64
	MTimeNS      int64
	VerifiedAt   int64
	KronkVersion string
}`}</code>
              </pre>
              <p className="doc-description">ArtifactVerification records a successful full-file verification of an artifact.</p>
            </div>

            <div className="doc-section" id="type-artifactverificationrecorder">
              <h4>ArtifactVerificationRecorder</h4>
              <pre className="code-block">
                <code>{`type ArtifactVerificationRecorder func(modelFile string, verification ArtifactVerification) error`}</code>
              </pre>
              <p className="doc-description">ArtifactVerificationRecorder persists a newly produced verification record. The model package does not prescribe where or how the record is stored.</p>
            </div>

            <div className="doc-section" id="type-batchenginesnapshot">
              <h4>BatchEngineSnapshot</h4>
              <pre className="code-block">
                <code>{`type BatchEngineSnapshot struct {
	Iteration               uint64
	PrefillBatchSize        int
	NBatch                  int
	NUBatch                 int
	MTP                     bool
	NDraft                  int
	QueuedRequests          int
	PendingRequests         int
	PrefillSelectorStart    int
	PrefillSelectorSelected int
	PrefillSelectorNext     int
	EligiblePrefillSlots    []int
	IMCSelectorStart        int
	IMCSelectorSelected     int
	IMCSelectorNext         int
	EligibleIMCSlots        []int
	GenerationRows          int
	PrefillRows             int
	TotalRows               int
	GenerationContributions []BatchGenerationContribution
	Slots                   []BatchSlotSnapshot
}`}</code>
              </pre>
              <p className="doc-description">BatchEngineSnapshot describes the latest immutable scheduler state published by a model's generation batch engine.</p>
            </div>

            <div className="doc-section" id="type-batchgenerationcontribution">
              <h4>BatchGenerationContribution</h4>
              <pre className="code-block">
                <code>{`type BatchGenerationContribution struct {
	SlotID int
	Rows   int
	Mode   string
}`}</code>
              </pre>
              <p className="doc-description">BatchGenerationContribution describes one slot's generation rows in the latest logical batch.</p>
            </div>

            <div className="doc-section" id="type-batchslotsnapshot">
              <h4>BatchSlotSnapshot</h4>
              <pre className="code-block">
                <code>{`type BatchSlotSnapshot struct {
	ID                      int
	Phase                   string
	RequestID               string
	RequestAge              time.Duration
	PrefillOwner            bool
	PromptTokens            int
	PrefilledTokens         int
	PrefillRemaining        int
	GeneratedTokens         int
	PastTokens              int
	GenerationMode          string
	GenerationRows          int
	IMCPreparedTokens       int
	IMCTotalTokens          int
	IMCPreparationRemaining int
}`}</code>
              </pre>
              <p className="doc-description">BatchSlotSnapshot describes one generation slot at a batch-loop boundary.</p>
            </div>

            <div className="doc-section" id="type-channel">
              <h4>Channel</h4>
              <pre className="code-block">
                <code>{`type Channel uint8`}</code>
              </pre>
              <p className="doc-description">Channel labels the semantic class of an emitted token, mapping 1:1 to the OpenAI chat/completions delta fields produced by the SSE writer.</p>
            </div>

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
	// Has unexported fields.
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

            <div className="doc-section" id="type-completiontokensdetails">
              <h4>CompletionTokensDetails</h4>
              <pre className="code-block">
                <code>{`type CompletionTokensDetails struct {
	ReasoningTokens int \`json:"reasoning_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">CompletionTokensDetails provides a breakdown of completion tokens.</p>
            </div>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	Adapters                   []AdapterConfig
	ArtifactIntegrity          map[string]ArtifactIntegrity
	PtrAdmissionTimeout        *time.Duration
	AutoTune                   bool
	AutoTuned                  bool
	PtrCacheMinTokens          *int
	CacheTypeK                 GGMLType
	CacheTypeV                 GGMLType
	PtrContextWindow           *int
	DefaultParams              Params
	ChatTemplateKwargs         D
	PtrDraftModel              *DraftModelConfig
	Devices                    []string // Device names for model execution (e.g., ["CUDA0", "CUDA1"])
	PtrFlashAttention          *FlashAttentionType
	PtrIMCSessionCapacity      *int
	PtrIncrementalCache        *bool
	PtrInsecureLogging         *bool
	JinjaFile                  string
	LoadMode                   LoadMode
	Log                        applog.Logger
	PtrMainGPU                 *int
	PtrMoE                     *MoEConfig
	ModelFiles                 []string
	PtrNGpuLayers              *int
	PtrNSeqMax                 *int
	PtrNThreads                *int
	PtrNThreadsBatch           *int
	PtrPrefillBatchSize        *int
	NUMA                       string
	PtrOffloadKQV              *bool
	PtrOpOffload               *bool
	PtrOpOffloadMinBatch       *int
	ProjFile                   string
	MTPDrafterFile             string
	PtrProjOnCPU               *bool
	ProjDevice                 string
	PtrQueueDepth              *int
	ResponseModelID            string
	PtrRopeFreqBase            *float32
	PtrRopeFreqScale           *float32
	RecordArtifactVerification ArtifactVerificationRecorder
	RopeScaling                RopeScalingType
	SessionStoreFactory        SessionStoreFactory
	Speculation                SpeculationMode
	PtrSplitMode               *SplitMode
	PtrSWAFull                 *bool
	TensorBuftOverrides        []string
	TensorSplit                []float32
	PtrYarnAttnFactor          *float32
	PtrYarnBetaFast            *float32
	PtrYarnBetaSlow            *float32
	PtrYarnExtFactor           *float32
	PtrYarnOrigCtx             *int

	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Config represents model level configuration. These values if configured incorrectly can cause the system to panic. The defaults are used when these values are set to 0. Adapters contains local llama.cpp-compatible LoRA adapter GGUF files to load with the model. Adapter scales are fixed for the lifetime of the model. ArtifactIntegrity contains expected artifact verification state keyed by model file path. It is used to avoid repeating completed verification work. AdmissionTimeout limits how long a request waits for an admission permit. The timeout applies only to admission, not request processing after a permit is acquired. When unset or set to 0, the default is 3 minutes. AutoTune, when true, asks kronk.New to run a hardware-aware analysis of the model (architecture, size, and available devices) and seed unset settings (context window, KV cache type, slots, flash attention, split mode, etc.) before loading. It is off by default for backwards compatibility, and any option the caller sets explicitly always wins over the analysis. It has no effect when using the low-level model package directly (only kronk.New applies it). AutoTuned records that an upstream owner such as the Kronk Model Server has already applied AutoTune. When AutoTune and AutoTuned are both true, kronk.New preserves the enabled state for diagnostics without repeating the hardware analysis. CacheMinTokens sets the minimum token count required before caching. Messages shorter than this threshold are not cached, as the overhead of cache management may outweigh the prefill savings. When set to 0, defaults to 100 tokens. CacheTypeK is the data type for the K (key) cache. This controls the precision of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0) reduce memory usage but may slightly affect quality. When left as the zero value (GGMLTypeAuto), the default llama.cpp value is used. CacheTypeV is the data type for the V (value) cache. This controls the precision of the value vectors in the KV cache. When left as the zero value (GGMLTypeAuto), the default llama.cpp value is used. ContextWindow (often referred to as context length) is the maximum number of tokens that a large language model can process and consider at one time when generating a response. It defines the model's effective "memory" for a single conversation or text generation task. When set to 0, the default value is 4096. DefaultParams contains the default sampling parameters for requests. ChatTemplateKwargs contains model-level defaults passed only to the Jinja chat template. Request-level chat_template_kwargs override matching keys. Resolved first-class request parameters remain top-level template values. PtrDraftModel configures a separate speculative-decoding draft model or an nDraft override for an auto-detected MTP head. Devices is a list of device names to use for model execution. When multiple devices are specified, the model is distributed across them according to the SplitMode and TensorSplit configuration. Device names can be obtained from the output of llama-bench --list-devices (e.g., "CUDA0", "CUDA1", "Metal"). When empty, the default device selection is used. PtrFlashAttention controls Flash Attention mode. Flash Attention reduces memory usage and speeds up attention computation, especially for large context windows. When nil, FlashAttentionAuto is used so llama.cpp can decide whether the active backend supports it. Set to FlashAttentionEnabled to force it on, or FlashAttentionDisabled to force it off. IMCSessionCapacity sets the number of reusable IMC session identities. When left unset or set to 0, generation models default to NSeqMax * max(3, QueueDepth). An explicit value must be at least NSeqMax * QueueDepth so every admitted generation request can reserve a session. IncrementalCache enables Incremental Message Caching (IMC) for agentic workflows. It caches all messages except the last one (which triggers generation) and extends the cache incrementally on each turn. This is ideal for agents like Cline or OpenCode where conversations grow monotonically. The cache is rebuilt from scratch when the message prefix changes (new thread). InsecureLogging enables logging of potentially sensitive data such as message content. This should only be enabled for debugging purposes in non-production environments. JinjaFile is the path to the jinja file. This is not required and can be used if you want to override the templated provided by the model metadata. LoadMode controls how model weights are loaded. The default is LoadModeAuto, which uses mmap when every selected device supports it and otherwise uses ordinary loading. LoadModeNone disables mmap, which can improve tensor placement on multi-socket NUMA systems running MoE models with CPU experts. LoadModeMLock requests resident pages without forcing mmap, LoadModeMMapMLock combines mmap and mlock, and LoadModeDirectIO bypasses the page cache where the platform and filesystem support it. Log is the logger to use for model operations. MainGPU is the index of the GPU to use as the primary device when SplitMode is SplitModeNone. When nil, the default GPU (usually index 0) is used. PtrMoE controls expert-tensor placement for Mixture of Experts models. ModelFiles is the path to the model files. This is mandatory to provide. PrefillBatchSize is the maximum number of prompt tokens one prefill owner can contribute to a decode iteration. The default is 2048. Larger values can reduce the number of decode calls needed to reach generation, but each call takes longer before already-generating slots can run again and requires larger compute buffers. Kronk derives llama.cpp's logical and physical batch capacities from this value, the configured slot count, and the generation mode. Multimodal models may require a complete media-token chunk to fit in this capacity. NGpuLayers is the number of model layers to offload to the GPU. When set to 0, all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any positive value specifies the exact number of layers to offload. NSeqMax controls concurrency behavior based on model type. For text inference models (including vision/audio), it sets the maximum number of generation slots. For supported embedding and reranking architectures, it sets the maximum sequence width of the sequence-batch engine. Other embedding and reranking architectures use it as the context-pool size. When set to 0, a default of 1 is used. NThreads is the number of threads to use for generation. When set to 0, the default llama.cpp value is used. NThreadsBatch is the number of threads to use for batch processing. When set to 0, the default llama.cpp value is used. NUMA controls the NUMA (Non-Uniform Memory Access) strategy. This matters most when expert tensors are on CPU and the system has multiple NUMA nodes. Valid values: "" (disabled), "distribute", "isolate", "numactl", "mirror". "distribute" is recommended for multi-socket MoE setups; without it, cross-socket memory access can cause significant bandwidth collapse. OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or true, the KV cache is stored on the GPU (default behavior). Set to false to keep the KV cache on the CPU, which reduces VRAM usage but may slow inference. OpOffload controls whether host tensor operations are offloaded to the device (GPU). When nil or true, operations are offloaded (default behavior). Set to false to keep operations on the CPU. OpOffloadMinBatch sets the minimum batch size at which host tensor operations are offloaded to the device. When unset or 0, llama.cpp's default is used. ProjFile is the path to the projection files. This is mandatory for media based models like vision and audio. MTPDrafterFile is the path to a separate-file MTP "assistant" drafter GGUF that ships alongside the main model (e.g. Gemma4's "mtp-gemma-4-26B-A4B-it-*.gguf"). It is NOT the main model and NOT a vocab-matched classic draft model: it is a per-model speculative head loaded as its own llama_model whose context shares the target's KV memory. Auto-wired from disk when the companion file is present; empty otherwise. Distinct from the embedded MTP head carried inside some target GGUFs (Qwen3.5/3.6), which has no separate file. ProjOnCPU forces the multimodal projector (mmproj) to run on the CPU. When nil or false, the projector runs on whichever device llama.cpp picks by default (GPU when available). Set to true to keep the projector on the CPU — equivalent to llama-mtmd-cli's --no-mmproj-offload. The LLM itself is unaffected and still runs on whatever device WithNGpuLayers selects. ProjDevice names the backend device used by the multimodal projector (mmproj), such as "CUDA1" or "MTL0". When empty, llama.cpp selects the projector device automatically. It cannot be combined with ProjOnCPU=true. The LLM device selection is unaffected. QueueDepth sets the multiplier for semaphore capacity when using the batch engine (NSeqMax &gt; 1). This controls how many requests can queue while the current batch is processing. Default is 2, meaning NSeqMax * 2 requests can be in-flight. Only applies to text inference models. RopeFreqBase overrides the RoPE base frequency. When nil, uses model default. Common values: 10000 (Llama), 1000000 (Qwen3). RopeFreqScale overrides the raw RoPE frequency multiplier. When nil, uses the value from model metadata. Kronk does not derive this value from ContextWindow; an N-times extension generally uses 1/N when the model's documentation requires explicit scaling. RecordArtifactVerification persists updated verification state after Kronk verifies a model artifact. When nil, verification state is not persisted. RopeScaling controls the RoPE scaling method for extended context support. Set to RopeScalingYaRN only when the model supports YaRN and configure the frequency scale required by that model. SessionStoreFactory constructs session stores for direct SDK use. Kronk invokes it separately for every working session store it needs and closes every successfully returned store. The factory must return a new, independent store on each call. System prompt preloads always remain in RAM. When nil, Kronk uses the built-in RAM factory. Backend-specific constructor parameters belong to the backend package and are captured by the injected factory. SplitMode controls how the model is split across multiple GPUs: - SplitModeNone (0): single GPU - SplitModeLayer (1): split layers and KV across GPUs - SplitModeRow (2): deprecated row-split tensor parallelism When nil (not set), the default is SplitModeLayer, matching llama.cpp. Layer mode distributes a single GGUF across multiple GPUs without requiring the backend-specific split buffers used by row mode. SWAFull controls whether models with sliding window attention (SWA) use a full-size KV cache for SWA layers instead of the memory-efficient small cache. When nil (default), llama.cpp's default is used. When explicitly set to false, SWA layers only cache the last n_swa tokens, saving significant VRAM but limiting context caching and shifting. When true, SWA layers use the full context window for their KV cache, preserving accuracy at the cost of higher memory usage. TensorBuftOverrides is a list of tensor buffer type override patterns that force matching tensors to execute on CPU instead of GPU. This is an expert-level configuration useful for MoE models where certain FFN expert tensors don't fit in VRAM. Supported values: - "all-ffn": offload all FFN expression tensors to CPU - "block:N": offload FFN tensors for block N to CPU (e.g., "block:12") - Any regex pattern matching tensor names (e.g., `blk\.12\.ffn_(up|down|gate)`) TensorSplit controls how model layers are proportionally distributed across multiple GPUs. Each element represents the fraction of the model assigned to the corresponding device. For example, [0.6, 0.4] splits 60%/40% across two GPUs. The length must match the number of devices. When empty, the split is determined automatically based on available VRAM. YarnAttnFactor sets the YaRN attention magnitude scaling factor. When nil, uses the model or llama.cpp default. YarnBetaFast sets the YaRN low correction dimension. When nil, uses the model or llama.cpp default. YarnBetaSlow sets the YaRN high correction dimension. When nil, uses the model or llama.cpp default. YarnExtFactor sets the YaRN extrapolation mix factor. When nil, uses the model or llama.cpp default. Set to 0 to disable extrapolation. YarnOrigCtx sets the original training context size for YaRN scaling. When nil or 0, uses the model's native training context length from metadata.</p>
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
	ModelFiles    []string  // Path to the draft model GGUF file(s); empty means MTP nDraft override
	NDraft        int       // Number of tokens to draft per step (separate-GGUF default 5, MTP default 3)
	PtrNGpuLayers *int      // GPU layers for draft model (nil = all layers on GPU)
	Devices       []string  // Devices for draft model (e.g., ["CUDA0"])
	PtrMainGPU    *int      // Primary GPU index for draft model
	TensorSplit   []float32 // Per-device tensor split for draft model
}`}</code>
              </pre>
              <p className="doc-description">DraftModelConfig configures speculative decoding for a target model. It serves two purposes depending on whether ModelFiles is set: 1. Separate-GGUF draft (ModelFiles set): a smaller, faster model generates candidate tokens that the target verifies in a single forward pass. Requires NSeqMax == 1 (single-slot mode) and a draft that shares the target's vocabulary (same tokenizer). 2. MTP nDraft override (ModelFiles empty): when the target GGUF ships an auto-detected MTP head, this block sets the number of draft tokens per round without supplying a separate model. NDraft defaults to defMTPNDraft when left unset. A model can have at most one drafter. If ModelFiles is set, the separate-GGUF drafter wins even on a target that also has an MTP head.</p>
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

            <div className="doc-section" id="type-fingerprint">
              <h4>Fingerprint</h4>
              <pre className="code-block">
                <code>{`type Fingerprint struct {
	ChatTemplate string // raw jinja chat template
	Architecture string // gguf "general.architecture" (e.g. "llama", "qwen2")
	ModelName    string // gguf "general.name" (e.g. "Qwen3-Coder-30B-A3B")
}`}</code>
              </pre>
              <p className="doc-description">Fingerprint carries the model metadata that parser selection logic inspects at Model.Load time.</p>
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

            <div className="doc-section" id="type-imcsessiondetail">
              <h4>IMCSessionDetail</h4>
              <pre className="code-block">
                <code>{`type IMCSessionDetail struct {
	ID             int
	State          IMCSessionState
	Context        int
	Allocated      int
	TotalAllocated int
	SnapshotBytes  int
	PeakContext    int
	Messages       int
	InputMessages  int
	InputTokens    int
	OutputTokens   int
	ContextWindow  int
	LastUsed       time.Time
	HasMedia       bool
}`}</code>
              </pre>
              <p className="doc-description">IMCSessionDetail is a scalar snapshot of one allocated IMC cache entry.</p>
            </div>

            <div className="doc-section" id="type-imcsessionstate">
              <h4>IMCSessionState</h4>
              <pre className="code-block">
                <code>{`type IMCSessionState string`}</code>
              </pre>
              <p className="doc-description">IMCSessionState describes the current state of an allocated IMC cache entry.</p>
            </div>

            <div className="doc-section" id="type-imcsystemcachedetail">
              <h4>IMCSystemCacheDetail</h4>
              <pre className="code-block">
                <code>{`type IMCSystemCacheDetail struct {
	ID             int
	Tokens         int
	Allocated      int
	SnapshotBytes  int
	RestoreCount   uint64
	ActiveRestores int
	LastUsed       time.Time
}`}</code>
              </pre>
              <p className="doc-description">IMCSystemCacheDetail is a scalar snapshot of one System cache pool entry.</p>
            </div>

            <div className="doc-section" id="type-loadmode">
              <h4>LoadMode</h4>
              <pre className="code-block">
                <code>{`type LoadMode int32`}</code>
              </pre>
              <p className="doc-description">LoadMode controls how model weights are loaded from storage.</p>
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
                <code>{`type MoEMode struct {
	// Has unexported fields.
}`}</code>
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
	FileType      int32
	Quantization  string
	VRAMTotal     int64
	SlotMemory    int64
	NSWA          int32 // Effective SWA window in tokens; zero means the model does not use SWA.
	Type          ModelType
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

	// DryPenaltyLast limits how many recent tokens DRY considers. A value of 0
	// disables DRY and -1 uses the full context.
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
	// streaming responses. Default is false.
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

	// ReasoningEffort requests a model-specific reasoning level. When empty, the
	// chat template determines its default.
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

	// Seed initializes sampling randomness. Nil reuses a matched IMC session's
	// seed or generates one; any non-nil value, including 0, requests repeatable
	// sampling and becomes the session's retained seed.
	Seed *uint32 \`json:"seed,omitempty"\`

	// Stream determines whether to stream the response.
	Stream bool \`json:"stream"\`

	// Stop contains sequences that terminate generation.
	Stop []string \`json:"stop,omitempty"\`

	// Temperature controls the randomness of the output. It rescales the
	// probability distribution of possible next tokens. Default is 1.0.
	Temperature float32 \`json:"temperature"\`

	// Thinking determines if the model should think or not. It is used for most
	// non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE,
	// false, False. Default is "true".
	Thinking string \`json:"enable_thinking"\`

	// TopK limits the pool of possible next tokens to the K number of most
	// probable tokens. If a model predicts 10,000 possible next tokens, setting
	// top_k to 50 means only the 50 tokens with the highest probabilities are
	// considered for selection (after temperature scaling). Default is 20.
	TopK int32 \`json:"top_k"\`

	// TopLogprobs specifies how many of the most likely tokens to return at
	// each position, along with their log probabilities. Must be between 0 and
	// 5. Setting this to a value > 0 implicitly enables logprobs. Default is 0.
	TopLogprobs int \`json:"top_logprobs"\`

	// TopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the
	// minimum number of most probable tokens required to reach the cumulative
	// probability P. Default is 0.95.
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

            <div className="doc-section" id="type-paramsadjuster">
              <h4>ParamsAdjuster</h4>
              <pre className="code-block">
                <code>{`type ParamsAdjuster interface {
	AdjustParams(p Params) Params
}`}</code>
              </pre>
              <p className="doc-description">ParamsAdjuster is an optional interface a Parser may implement to coerce request Params into values its model lineage's chat template will accept. It is invoked at the end of Model.adjustParams, after global defaults have been applied. Use cases include clamping reasoning_effort to the subset of values a strict template (e.g. Mistral Medium 3.5) will validate.</p>
            </div>

            <div className="doc-section" id="type-parser">
              <h4>Parser</h4>
              <pre className="code-block">
                <code>{`type Parser interface {
	// Name returns the parser identifier (e.g. "fallback", "gpt-oss").
	// Used for logging and as the override key in model configs.
	Name() string

	// NewStateMachine returns a fresh per-slot state machine. Callers must
	// not share StateMachine instances across slots.
	NewStateMachine() StateMachine

	// ToolCall parses the accumulated tool-call buffer into structured
	// tool calls. Called once when generation finishes, never on the hot
	// per-token path. The logger is used for repair/parse failures; tests
	// may pass a no-op logger.
	ToolCall(ctx context.Context, log applog.Logger, buf string) []ResponseToolCall
}`}</code>
              </pre>
              <p className="doc-description">Parser is the plugin interface implemented by each model lineage. Implementations live in sdk/kronk/parsers/&lt;name&gt;/ and are registered at startup via RegisterParser.</p>
            </div>

            <div className="doc-section" id="type-parserfactory">
              <h4>ParserFactory</h4>
              <pre className="code-block">
                <code>{`type ParserFactory func(Fingerprint) (Parser, bool)`}</code>
              </pre>
              <p className="doc-description">ParserFactory is the constructor signature each parser package's New function satisfies. The bool return reports whether this parser claims the given Fingerprint; on false, the registry continues to the next factory.</p>
            </div>

            <div className="doc-section" id="type-prompttokensdetails">
              <h4>PromptTokensDetails</h4>
              <pre className="code-block">
                <code>{`type PromptTokensDetails struct {
	CachedTokens int \`json:"cached_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">PromptTokensDetails provides a breakdown of prompt tokens.</p>
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
	Role           string                  \`json:"role,omitempty"\`
	Content        string                  \`json:"content"\`
	Reasoning      string                  \`json:"reasoning_content,omitempty"\`
	ToolCalls      []ResponseToolCall      \`json:"tool_calls,omitempty"\`
	ToolCallDeltas []ResponseToolCallDelta \`json:"-"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseMessage represents a single message in a response.</p>
            </div>

            <div className="doc-section" id="type-responsetoolcall">
              <h4>ResponseToolCall</h4>
              <pre className="code-block">
                <code>{`type ResponseToolCall struct {
	ID       string                   \`json:"id"\`
	Index    int                      \`json:"-"\`
	Type     string                   \`json:"type"\`
	Function ResponseToolCallFunction \`json:"function"\`
	Status   int                      \`json:"status,omitempty"\`
	Raw      string                   \`json:"raw,omitempty"\`
	Error    string                   \`json:"error,omitempty"\`
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="type-responsetoolcalldelta">
              <h4>ResponseToolCallDelta</h4>
              <pre className="code-block">
                <code>{`type ResponseToolCallDelta struct {
	ID       string                        \`json:"id,omitempty"\`
	Index    int                           \`json:"index"\`
	Type     string                        \`json:"type,omitempty"\`
	Function ResponseToolCallDeltaFunction \`json:"function"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseToolCallDelta represents an incremental OpenAI tool-call delta.</p>
            </div>

            <div className="doc-section" id="type-responsetoolcalldeltafunction">
              <h4>ResponseToolCallDeltaFunction</h4>
              <pre className="code-block">
                <code>{`type ResponseToolCallDeltaFunction struct {
	Name      string \`json:"name,omitempty"\`
	Arguments string \`json:"arguments"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseToolCallDeltaFunction represents an incremental function-call delta.</p>
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

            <div className="doc-section" id="type-result">
              <h4>Result</h4>
              <pre className="code-block">
                <code>{`type Result struct {
	Channel Channel
	Content string
}`}</code>
              </pre>
              <p className="doc-description">Result is the per-token outcome returned by StateMachine.Classify. Content may be empty when the token is a structural marker that has been fully consumed by the state machine (e.g. &lt;think&gt;, &lt;tool_call&gt;). When Content is non-empty, it is routed to the appropriate accumulator based on Channel.</p>
            </div>

            <div className="doc-section" id="type-ropescalingtype">
              <h4>RopeScalingType</h4>
              <pre className="code-block">
                <code>{`type RopeScalingType int32`}</code>
              </pre>
              <p className="doc-description">RopeScalingType controls RoPE (Rotary Position Embedding) scaling method. This enables extended context windows beyond the model's native training length. For example, Qwen3 models trained on 32k can support 131k with YaRN scaling.</p>
            </div>

            <div className="doc-section" id="type-sessionstore">
              <h4>SessionStore</h4>
              <pre className="code-block">
                <code>{`type SessionStore = kvstorage.Store`}</code>
              </pre>
              <p className="doc-description">SessionStore is the storage contract used for an externalized IMC session.</p>
            </div>

            <div className="doc-section" id="type-sessionstorefactory">
              <h4>SessionStoreFactory</h4>
              <pre className="code-block">
                <code>{`type SessionStoreFactory = kvstorage.Factory`}</code>
              </pre>
              <p className="doc-description">SessionStoreFactory constructs an independent SessionStore.</p>
            </div>

            <div className="doc-section" id="type-speculationmode">
              <h4>SpeculationMode</h4>
              <pre className="code-block">
                <code>{`type SpeculationMode = internalspec.Mode`}</code>
              </pre>
              <p className="doc-description">SpeculationMode selects the speculative-decoding implementation for a model.</p>
            </div>

            <div className="doc-section" id="type-splitmode">
              <h4>SplitMode</h4>
              <pre className="code-block">
                <code>{`type SplitMode int32`}</code>
              </pre>
              <p className="doc-description">SplitMode controls how the model is split across multiple GPUs.</p>
            </div>

            <div className="doc-section" id="type-statemachine">
              <h4>StateMachine</h4>
              <pre className="code-block">
                <code>{`type StateMachine interface {
	// Classify classifies a single decoded token's content and returns the
	// Result plus whether the model has signaled end-of-generation.
	Classify(content string) (r Result, eog bool)

	// Reset returns the state machine to its initial state for reuse.
	Reset()
}`}</code>
              </pre>
              <p className="doc-description">StateMachine is the per-request, per-slot streaming state machine. One instance is created per slot via Parser.NewStateMachine and reused across requests on that slot via Reset. Behavior is undefined if Classify is called after a previous call returned eog=true. Callers must invoke Reset before reusing the state machine.</p>
            </div>

            <div className="doc-section" id="type-statemachineflusher">
              <h4>StateMachineFlusher</h4>
              <pre className="code-block">
                <code>{`type StateMachineFlusher interface {
	Flush() Result
}`}</code>
              </pre>
              <p className="doc-description">StateMachineFlusher is implemented by state machines that may retain model output not yet returned by Classify. Flush drains that output at successful end-of-generation. It must not return content previously returned by Classify. Callers invoke Flush until it returns a zero Result so state machines can preserve multiple channel transitions retained from one decoded piece.</p>
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

            <div className="doc-section" id="type-toolawarestatemachine">
              <h4>ToolAwareStateMachine</h4>
              <pre className="code-block">
                <code>{`type ToolAwareStateMachine interface {
	SetTools(tools []D)
}`}</code>
              </pre>
              <p className="doc-description">ToolAwareStateMachine is optionally implemented by state machines that need the request's declared tools to distinguish an unmarked tool call from ordinary answer content.</p>
            </div>

            <div className="doc-section" id="type-toolcallarguments">
              <h4>ToolCallArguments</h4>
              <pre className="code-block">
                <code>{`type ToolCallArguments map[string]any`}</code>
              </pre>
              <p className="doc-description">ToolCallArguments represents tool call arguments that marshal to a JSON string per OpenAI API spec, but can unmarshal from either a string or object.</p>
            </div>

            <div className="doc-section" id="type-toolcalldeltastreamer">
              <h4>ToolCallDeltaStreamer</h4>
              <pre className="code-block">
                <code>{`type ToolCallDeltaStreamer interface {
	ToolCallDeltas() []ResponseToolCallDelta

	// StartedToolCalls returns the tool-call identities emitted during the
	// current request. Callers must not modify the returned slice.
	StartedToolCalls() []ResponseToolCallDelta
}`}</code>
              </pre>
              <p className="doc-description">ToolCallDeltaStreamer is implemented by state machines that can translate model-native tool-call starts into OpenAI-compatible activity deltas. ToolCallDeltas drains deltas produced by the most recent Classify call.</p>
            </div>

            <div className="doc-section" id="type-toolcallschemaparser">
              <h4>ToolCallSchemaParser</h4>
              <pre className="code-block">
                <code>{`type ToolCallSchemaParser interface {
	ToolCallWithSchema(ctx context.Context, log applog.Logger, buf string, tools []D) []ResponseToolCall
}`}</code>
              </pre>
              <p className="doc-description">ToolCallSchemaParser is optionally implemented by parsers whose native tool call format does not encode argument types. The request's tool declarations are supplied so the parser can convert raw values according to their schema.</p>
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
	PromptTokens            int                     \`json:"prompt_tokens"\`
	PromptTokensDetails     PromptTokensDetails     \`json:"prompt_tokens_details"\`
	CompletionTokens        int                     \`json:"completion_tokens"\`
	CompletionTokensDetails CompletionTokensDetails \`json:"completion_tokens_details"\`
	TotalTokens             int                     \`json:"total_tokens"\`
	TokensPerSecond         float64                 \`json:"tokens_per_second"\`
	TimeToFirstTokenMS      float64                 \`json:"time_to_first_token_ms"\`
	DraftTokens             int                     \`json:"draft_tokens,omitempty"\`
	DraftAcceptedTokens     int                     \`json:"draft_accepted_tokens,omitempty"\`
	DraftAcceptanceRate     float64                 \`json:"draft_acceptance_rate,omitempty"\`
	DraftCoverage           float64                 \`json:"draft_coverage,omitempty"\`
	DraftDisableReason      string                  \`json:"draft_disable_reason,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">Usage provides token usage information for a chat completion request. CompletionTokens includes all generated tokens, including reasoning tokens. CompletionTokensDetails provides the reasoning-token subset. TotalTokens is the sum of PromptTokens and CompletionTokens. DraftAcceptanceRate is the ratio of accepted drafts to total drafts across the spec rounds that actually ran. It is "quality per round" and says nothing about how much of the request used speculation. DraftCoverage is the complementary "how much" metric: the fraction of emitted output positions produced through speculation. Together they distinguish "MTP ran the whole request at 94%" from "MTP ran for 4 rounds at 94% then was disabled and the rest was target-only" — the second case shows high DraftAcceptanceRate but low DraftCoverage. DraftDisableReason explains the latter case ("imc-hit", "mirror-error", or empty if MTP was never disabled).</p>
            </div>

            <div className="doc-section" id="type-vocabeogconsumer">
              <h4>VocabEOGConsumer</h4>
              <pre className="code-block">
                <code>{`type VocabEOGConsumer interface {
	// ConsumeVocabEOG consumes the textual representation of the sampled EOG
	// token before the state machine is flushed.
	ConsumeVocabEOG(content string)
}`}</code>
              </pre>
              <p className="doc-description">VocabEOGConsumer is optionally implemented by state machines whose native framing uses a token that the vocabulary also classifies as end-of-generation.</p>
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

            <div className="doc-section" id="method-config-admissiontimeout">
              <h4>Config.AdmissionTimeout</h4>
              <pre className="code-block">
                <code>func (cfg Config) AdmissionTimeout() time.Duration</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-cachemintokens">
              <h4>Config.CacheMinTokens</h4>
              <pre className="code-block">
                <code>func (cfg Config) CacheMinTokens() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-contextwindow">
              <h4>Config.ContextWindow</h4>
              <pre className="code-block">
                <code>func (cfg Config) ContextWindow() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-effectivenbatch">
              <h4>Config.EffectiveNBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) EffectiveNBatch() int</code>
              </pre>
              <p className="doc-description">EffectiveNBatch returns the derived llama.cpp logical batch capacity.</p>
            </div>

            <div className="doc-section" id="method-config-effectivenubatch">
              <h4>Config.EffectiveNUBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) EffectiveNUBatch() int</code>
              </pre>
              <p className="doc-description">EffectiveNUBatch returns the derived llama.cpp physical batch capacity.</p>
            </div>

            <div className="doc-section" id="method-config-expertlayersongpu">
              <h4>Config.ExpertLayersOnGPU</h4>
              <pre className="code-block">
                <code>func (cfg Config) ExpertLayersOnGPU() int64</code>
              </pre>
              <p className="doc-description">ExpertLayersOnGPU translates the model's MoE configuration into the value the vram calculator expects so every prediction site (resman planner, post-load model logging, BUI display) reflects what llama.cpp will actually do at runtime. With no MoE override we mirror llama.cpp's default behavior: experts follow the layer they belong to, and full GPU offload puts every expert on the GPU. Without this resolution the calculator defaults experts to CPU even when the runtime puts them on GPU, producing the inverse of the placement that's actually loaded and silently under-accounting expert weight memory in the BUI VRAM column.</p>
            </div>

            <div className="doc-section" id="method-config-flashattention">
              <h4>Config.FlashAttention</h4>
              <pre className="code-block">
                <code>func (cfg Config) FlashAttention() FlashAttentionType</code>
              </pre>
              <p className="doc-description">FlashAttention returns the configured flash attention mode. An unset value defaults to auto so llama.cpp can select the supported mode.</p>
            </div>

            <div className="doc-section" id="method-config-imcsessioncapacity">
              <h4>Config.IMCSessionCapacity</h4>
              <pre className="code-block">
                <code>func (cfg Config) IMCSessionCapacity() int</code>
              </pre>
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

            <div className="doc-section" id="method-config-opoffloadminbatch">
              <h4>Config.OpOffloadMinBatch</h4>
              <pre className="code-block">
                <code>func (cfg Config) OpOffloadMinBatch() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-config-prefillbatchsize">
              <h4>Config.PrefillBatchSize</h4>
              <pre className="code-block">
                <code>func (cfg Config) PrefillBatchSize() int</code>
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

            <div className="doc-section" id="method-config-swafull">
              <h4>Config.SWAFull</h4>
              <pre className="code-block">
                <code>func (cfg Config) SWAFull() bool</code>
              </pre>
              <p className="doc-description">SWAFull reports whether a full-size sliding-window attention cache was explicitly requested. Callers that need the effective value must check PtrSWAFull first because nil leaves the choice to llama.cpp.</p>
            </div>

            <div className="doc-section" id="method-config-speculationmode">
              <h4>Config.SpeculationMode</h4>
              <pre className="code-block">
                <code>func (cfg Config) SpeculationMode() SpeculationMode</code>
              </pre>
              <p className="doc-description">SpeculationMode returns the selected speculative-decoding implementation. The zero value preserves automatic selection.</p>
            </div>

            <div className="doc-section" id="method-config-string">
              <h4>Config.String</h4>
              <pre className="code-block">
                <code>func (cfg Config) String() string</code>
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
              <p className="doc-description">Clone creates a copy of the document. Mutable JSON maps and slices are cloned recursively so callers can reuse a request after submitting it. Byte slices are shared as immutable media payloads; scalar values are shared by value.</p>
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

            <div className="doc-section" id="method-draftmodelconfig-isseparate">
              <h4>DraftModelConfig.IsSeparate</h4>
              <pre className="code-block">
                <code>func (d DraftModelConfig) IsSeparate() bool</code>
              </pre>
              <p className="doc-description">IsSeparate reports whether this config points at a separate draft GGUF. When false, the config is an MTP nDraft override that carries no model files and only applies when the target ships an auto-detected MTP head.</p>
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

            <div className="doc-section" id="method-loadmode-marshaljson">
              <h4>LoadMode.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (lm LoadMode) MarshalJSON() ([]byte, error)</code>
              </pre>
              <p className="doc-description">MarshalJSON implements json.Marshaler.</p>
            </div>

            <div className="doc-section" id="method-loadmode-marshalyaml">
              <h4>LoadMode.MarshalYAML</h4>
              <pre className="code-block">
                <code>func (lm LoadMode) MarshalYAML() (any, error)</code>
              </pre>
              <p className="doc-description">MarshalYAML implements yaml.Marshaler.</p>
            </div>

            <div className="doc-section" id="method-loadmode-string">
              <h4>LoadMode.String</h4>
              <pre className="code-block">
                <code>func (lm LoadMode) String() string</code>
              </pre>
              <p className="doc-description">String returns the string representation of a LoadMode.</p>
            </div>

            <div className="doc-section" id="method-loadmode-toyzmatype">
              <h4>LoadMode.ToYZMAType</h4>
              <pre className="code-block">
                <code>func (lm LoadMode) ToYZMAType() llama.LoadMode</code>
              </pre>
              <p className="doc-description">ToYZMAType converts to the yzma/llama.cpp LoadMode type.</p>
            </div>

            <div className="doc-section" id="method-loadmode-unmarshaljson">
              <h4>LoadMode.UnmarshalJSON</h4>
              <pre className="code-block">
                <code>func (lm *LoadMode) UnmarshalJSON(data []byte) error</code>
              </pre>
              <p className="doc-description">UnmarshalJSON implements json.Unmarshaler.</p>
            </div>

            <div className="doc-section" id="method-loadmode-unmarshalyaml">
              <h4>LoadMode.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (lm *LoadMode) UnmarshalYAML(unmarshal func(any) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler.</p>
            </div>

            <div className="doc-section" id="method-moeconfig-keepexpertsongpufortopnlayers">
              <h4>MoEConfig.KeepExpertsOnGPUForTopNLayers</h4>
              <pre className="code-block">
                <code>func (m MoEConfig) KeepExpertsOnGPUForTopNLayers() int</code>
              </pre>
            </div>

            <div className="doc-section" id="method-moemode-equal">
              <h4>MoEMode.Equal</h4>
              <pre className="code-block">
                <code>func (mm MoEMode) Equal(mm2 MoEMode) bool</code>
              </pre>
              <p className="doc-description">Equal provides support for the go-cmp package and testing.</p>
            </div>

            <div className="doc-section" id="method-moemode-iszero">
              <h4>MoEMode.IsZero</h4>
              <pre className="code-block">
                <code>func (mm MoEMode) IsZero() bool</code>
              </pre>
              <p className="doc-description">IsZero reports whether the MoE mode is unset.</p>
            </div>

            <div className="doc-section" id="method-moemode-marshaltext">
              <h4>MoEMode.MarshalText</h4>
              <pre className="code-block">
                <code>func (mm MoEMode) MarshalText() ([]byte, error)</code>
              </pre>
              <p className="doc-description">MarshalText provides support for logging and serialization.</p>
            </div>

            <div className="doc-section" id="method-moemode-string">
              <h4>MoEMode.String</h4>
              <pre className="code-block">
                <code>func (mm MoEMode) String() string</code>
              </pre>
              <p className="doc-description">String returns the name of the MoE mode.</p>
            </div>

            <div className="doc-section" id="method-moemode-unmarshaltext">
              <h4>MoEMode.UnmarshalText</h4>
              <pre className="code-block">
                <code>func (mm *MoEMode) UnmarshalText(data []byte) error</code>
              </pre>
              <p className="doc-description">UnmarshalText parses serialized text into a known MoEMode.</p>
            </div>

            <div className="doc-section" id="method-model-batchenginesnapshot">
              <h4>Model.BatchEngineSnapshot</h4>
              <pre className="code-block">
                <code>func (m *Model) BatchEngineSnapshot() (BatchEngineSnapshot, bool)</code>
              </pre>
              <p className="doc-description">BatchEngineSnapshot returns the latest generation scheduler snapshot. Models used only for embeddings or reranking do not have a generation batch engine.</p>
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
                <code>func (m *Model) ChatStreaming(ctx context.Context, d D) (&lt;-chan ChatResponse, error)</code>
              </pre>
              <p className="doc-description">ChatStreaming performs a chat request and streams the response. All requests (including vision/audio) use batch processing and can run concurrently based on the NSeqMax config value, which controls parallel sequence processing. When stream_options.include_usage is true, the terminal choice is followed by a usage response with an empty Choices slice. Validation failures are returned before a response channel is created.</p>
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
                <code>func (m *Model) Embeddings(ctx context.Context, d D) (response EmbedReponse, err error)</code>
              </pre>
              <p className="doc-description">Embeddings performs embedding for one or more inputs. Supported options in d: - input ([]string): the texts to embed (required) - truncate (bool): if true, truncate inputs to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models) Supported models process inputs together as a multi-sequence batch. Other models use the context-pool fallback.</p>
            </div>

            <div className="doc-section" id="method-model-imcsessions">
              <h4>Model.IMCSessions</h4>
              <pre className="code-block">
                <code>func (m *Model) IMCSessions() []IMCSessionDetail</code>
              </pre>
              <p className="doc-description">IMCSessions returns the current state of the model's allocated IMC cache entries. It does not retain history or expose the cached content.</p>
            </div>

            <div className="doc-section" id="method-model-imcsystemcaches">
              <h4>Model.IMCSystemCaches</h4>
              <pre className="code-block">
                <code>func (m *Model) IMCSystemCaches() []IMCSystemCacheDetail</code>
              </pre>
              <p className="doc-description">IMCSystemCaches returns every entry in the immutable System preload pool.</p>
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
                <code>func (m *Model) Rerank(ctx context.Context, d D) (response RerankResponse, err error)</code>
              </pre>
              <p className="doc-description">Rerank performs reranking for a query against multiple documents. It scores each document's relevance to the query and returns results sorted by relevance score (highest first). Supported options in d: - query (string): the query to rank documents against (required) - documents ([]string): the documents to rank (required) - top_n (int): return only the top N results (optional, default: all) - return_documents (bool): include document text in results (default: false) Supported models process documents together as a multi-sequence batch. Other models use the context-pool fallback.</p>
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
              <p className="doc-description">String returns a string representation of all resolved Params values in the format key[value]\nkey[value]\n ... Grammar contents are intentionally redacted; only whether a grammar is active is reported.</p>
            </div>

            <div className="doc-section" id="method-responsemessage-marshaljson">
              <h4>ResponseMessage.MarshalJSON</h4>
              <pre className="code-block">
                <code>func (m ResponseMessage) MarshalJSON() ([]byte, error)</code>
              </pre>
              <p className="doc-description">MarshalJSON emits incremental and completed tool calls through the same OpenAI-compatible tool_calls wire field.</p>
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
	FinishReasonStop   = "stop"
	FinishReasonLength = "length"
	FinishReasonTool   = "tool_calls"
	FinishReasonError  = "error"
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

	// DefDryPenaltyLast limits how many recent tokens DRY considers. A value
	// of -1 uses the full context.
	DefDryPenaltyLast = -1

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
	DefIncludeUsage = false

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

	// DefTemp controls the randomness of the output. It rescales the probability
	// distribution of possible next tokens.
	DefTemp = 1.0

	// DefTopK limits the pool of possible next tokens to the K number of most
	// probable tokens.
	DefTopK int32 = 20

	// DefTopLogprobs specifies how many of the most likely tokens to return at each
	// position, along with their log probabilities. Must be between 0 and 5.
	// Setting this to a value > 0 implicitly enables logprobs.
	DefTopLogprobs = 0

	// DefTopP, also known as nucleus sampling, works differently than top_k by
	// selecting a dynamic pool of tokens whose cumulative probability exceeds a
	// threshold P. Instead of a fixed number of tokens (K), it selects the minimum
	// number of most probable tokens required to reach the cumulative probability P.
	DefTopP = 0.95

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

            <div className="doc-section" id="const-speculationauto">
              <h4>SpeculationAuto</h4>
              <pre className="code-block">
                <code>{`const (
	SpeculationAuto     = internalspec.ModeAuto
	SpeculationDisabled = internalspec.ModeDisabled
	SpeculationClassic  = internalspec.ModeClassic
	SpeculationMTP      = internalspec.ModeMTP
)`}</code>
              </pre>
            </div>

            <div className="doc-section" id="const-defaultprefillbatchsize">
              <h4>DefaultPrefillBatchSize</h4>
              <pre className="code-block">
                <code>{`const DefaultPrefillBatchSize = int(vram.DefaultNUBatch)`}</code>
              </pre>
              <p className="doc-description">DefaultPrefillBatchSize is the prompt-token capacity used when prefill batch size is not explicitly configured.</p>
            </div>

            <div className="doc-section" id="const-defaultqueuedepth">
              <h4>DefaultQueueDepth</h4>
              <pre className="code-block">
                <code>{`const DefaultQueueDepth = 2`}</code>
              </pre>
              <p className="doc-description">DefaultQueueDepth is the admission-capacity multiplier used when queue depth is not explicitly configured.</p>
            </div>

            <div className="doc-section" id="const-expertsallongpu">
              <h4>ExpertsAllOnGPU</h4>
              <pre className="code-block">
                <code>{`const ExpertsAllOnGPU = int64(math.MaxInt32)`}</code>
              </pre>
              <p className="doc-description">ExpertsAllOnGPU is the sentinel value used for vram.Config.ExpertLayersOnGPU to request that every routed-expert layer be charged against GPU VRAM. The vram package treats any value greater than or equal to the model's block count as "all layers on GPU", so a large constant works regardless of model depth and avoids a metadata round-trip just to discover it.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-errfileinputsunsupported">
              <h4>ErrFileInputsUnsupported</h4>
              <pre className="code-block">
                <code>{`var ErrFileInputsUnsupported = errors.New("file inputs are not currently supported")`}</code>
              </pre>
              <p className="doc-description">ErrFileInputsUnsupported indicates file content parts are not supported.</p>
            </div>

            <div className="doc-section" id="var-errinvalidrequest">
              <h4>ErrInvalidRequest</h4>
              <pre className="code-block">
                <code>{`var ErrInvalidRequest = errors.New("validate-document: invalid request")`}</code>
              </pre>
              <p className="doc-description">ErrInvalidRequest indicates that a model request contains an invalid or unsupported field value.</p>
            </div>

            <div className="doc-section" id="var-errmessagesinvalid">
              <h4>ErrMessagesInvalid</h4>
              <pre className="code-block">
                <code>{`var ErrMessagesInvalid = errors.New("validate-document: messages is not a slice of documents")`}</code>
              </pre>
              <p className="doc-description">ErrMessagesInvalid indicates that a chat request's messages field has an invalid type.</p>
            </div>

            <div className="doc-section" id="var-errmessagesmissing">
              <h4>ErrMessagesMissing</h4>
              <pre className="code-block">
                <code>{`var ErrMessagesMissing = errors.New("validate-document: no messages found in request")`}</code>
              </pre>
              <p className="doc-description">ErrMessagesMissing indicates that a chat request has no messages field.</p>
            </div>

            <div className="doc-section" id="var-moemodeauto">
              <h4>MoEModeAuto</h4>
              <pre className="code-block">
                <code>{`var MoEModeAuto = newMoEMode("auto")`}</code>
              </pre>
              <p className="doc-description">MoEModeAuto uses catalog defaults.</p>
            </div>

            <div className="doc-section" id="var-moemodecustom">
              <h4>MoEModeCustom</h4>
              <pre className="code-block">
                <code>{`var MoEModeCustom = newMoEMode("custom")`}</code>
              </pre>
              <p className="doc-description">MoEModeCustom defers to TensorBuftOverrides for expert placement.</p>
            </div>

            <div className="doc-section" id="var-moemodeexpertscpu">
              <h4>MoEModeExpertsCPU</h4>
              <pre className="code-block">
                <code>{`var MoEModeExpertsCPU = newMoEMode("experts_cpu")`}</code>
              </pre>
              <p className="doc-description">MoEModeExpertsCPU places all routed expert tensors on CPU. Recommended for VRAM-constrained setups.</p>
            </div>

            <div className="doc-section" id="var-moemodeexpertsgpu">
              <h4>MoEModeExpertsGPU</h4>
              <pre className="code-block">
                <code>{`var MoEModeExpertsGPU = newMoEMode("experts_gpu")`}</code>
              </pre>
              <p className="doc-description">MoEModeExpertsGPU keeps all expert tensors on GPU. Requires sufficient VRAM for the full model.</p>
            </div>

            <div className="doc-section" id="var-moemodekeeptopn">
              <h4>MoEModeKeepTopN</h4>
              <pre className="code-block">
                <code>{`var MoEModeKeepTopN = newMoEMode("keep_top_n")`}</code>
              </pre>
              <p className="doc-description">MoEModeKeepTopN keeps routed experts on GPU for the top N layers. All other expert layers go to CPU.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-addparams">AddParams</a></li>
                <li><a href="#func-detectmodeltypefromfiles">DetectModelTypeFromFiles</a></li>
                <li><a href="#func-getembeddingsprenorm">GetEmbeddingsPreNorm</a></li>
                <li><a href="#func-getembeddingsprenormith">GetEmbeddingsPreNormIth</a></li>
                <li><a href="#func-inityzmaworkarounds">InitYzmaWorkarounds</a></li>
                <li><a href="#func-mtpavailable">MTPAvailable</a></li>
                <li><a href="#func-newmodel">NewModel</a></li>
                <li><a href="#func-parseggmltype">ParseGGMLType</a></li>
                <li><a href="#func-parseloadmode">ParseLoadMode</a></li>
                <li><a href="#func-parsemoemode">ParseMoEMode</a></li>
                <li><a href="#func-parseropescalingtype">ParseRopeScalingType</a></li>
                <li><a href="#func-parsesplitmode">ParseSplitMode</a></li>
                <li><a href="#func-recurrentstatecopies">RecurrentStateCopies</a></li>
                <li><a href="#func-registerparser">RegisterParser</a></li>
                <li><a href="#func-setembeddingsprenorm">SetEmbeddingsPreNorm</a></li>
                <li><a href="#func-validatechatrequest">ValidateChatRequest</a></li>
                <li><a href="#func-validatemessages">ValidateMessages</a></li>
                <li><a href="#func-verifyartifact">VerifyArtifact</a></li>
                <li><a href="#func-newgrammarsampler">NewGrammarSampler</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-adapterconfig">AdapterConfig</a></li>
                <li><a href="#type-artifactdigest">ArtifactDigest</a></li>
                <li><a href="#type-artifactintegrity">ArtifactIntegrity</a></li>
                <li><a href="#type-artifactverification">ArtifactVerification</a></li>
                <li><a href="#type-artifactverificationrecorder">ArtifactVerificationRecorder</a></li>
                <li><a href="#type-batchenginesnapshot">BatchEngineSnapshot</a></li>
                <li><a href="#type-batchgenerationcontribution">BatchGenerationContribution</a></li>
                <li><a href="#type-batchslotsnapshot">BatchSlotSnapshot</a></li>
                <li><a href="#type-channel">Channel</a></li>
                <li><a href="#type-chatresponse">ChatResponse</a></li>
                <li><a href="#type-choice">Choice</a></li>
                <li><a href="#type-completiontokensdetails">CompletionTokensDetails</a></li>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-contentlogprob">ContentLogprob</a></li>
                <li><a href="#type-d">D</a></li>
                <li><a href="#type-draftmodelconfig">DraftModelConfig</a></li>
                <li><a href="#type-embeddata">EmbedData</a></li>
                <li><a href="#type-embedreponse">EmbedReponse</a></li>
                <li><a href="#type-embedusage">EmbedUsage</a></li>
                <li><a href="#type-fingerprint">Fingerprint</a></li>
                <li><a href="#type-flashattentiontype">FlashAttentionType</a></li>
                <li><a href="#type-ggmltype">GGMLType</a></li>
                <li><a href="#type-imcsessiondetail">IMCSessionDetail</a></li>
                <li><a href="#type-imcsessionstate">IMCSessionState</a></li>
                <li><a href="#type-imcsystemcachedetail">IMCSystemCacheDetail</a></li>
                <li><a href="#type-loadmode">LoadMode</a></li>
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
                <li><a href="#type-paramsadjuster">ParamsAdjuster</a></li>
                <li><a href="#type-parser">Parser</a></li>
                <li><a href="#type-parserfactory">ParserFactory</a></li>
                <li><a href="#type-prompttokensdetails">PromptTokensDetails</a></li>
                <li><a href="#type-rerankresponse">RerankResponse</a></li>
                <li><a href="#type-rerankresult">RerankResult</a></li>
                <li><a href="#type-rerankusage">RerankUsage</a></li>
                <li><a href="#type-responsemessage">ResponseMessage</a></li>
                <li><a href="#type-responsetoolcall">ResponseToolCall</a></li>
                <li><a href="#type-responsetoolcalldelta">ResponseToolCallDelta</a></li>
                <li><a href="#type-responsetoolcalldeltafunction">ResponseToolCallDeltaFunction</a></li>
                <li><a href="#type-responsetoolcallfunction">ResponseToolCallFunction</a></li>
                <li><a href="#type-result">Result</a></li>
                <li><a href="#type-ropescalingtype">RopeScalingType</a></li>
                <li><a href="#type-sessionstore">SessionStore</a></li>
                <li><a href="#type-sessionstorefactory">SessionStoreFactory</a></li>
                <li><a href="#type-speculationmode">SpeculationMode</a></li>
                <li><a href="#type-splitmode">SplitMode</a></li>
                <li><a href="#type-statemachine">StateMachine</a></li>
                <li><a href="#type-statemachineflusher">StateMachineFlusher</a></li>
                <li><a href="#type-streamingresponselogger">StreamingResponseLogger</a></li>
                <li><a href="#type-template">Template</a></li>
                <li><a href="#type-tokenizeresponse">TokenizeResponse</a></li>
                <li><a href="#type-toolawarestatemachine">ToolAwareStateMachine</a></li>
                <li><a href="#type-toolcallarguments">ToolCallArguments</a></li>
                <li><a href="#type-toolcalldeltastreamer">ToolCallDeltaStreamer</a></li>
                <li><a href="#type-toolcallschemaparser">ToolCallSchemaParser</a></li>
                <li><a href="#type-toplogprob">TopLogprob</a></li>
                <li><a href="#type-usage">Usage</a></li>
                <li><a href="#type-vocabeogconsumer">VocabEOGConsumer</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-choice-finishreason">Choice.FinishReason</a></li>
                <li><a href="#method-config-admissiontimeout">Config.AdmissionTimeout</a></li>
                <li><a href="#method-config-cachemintokens">Config.CacheMinTokens</a></li>
                <li><a href="#method-config-contextwindow">Config.ContextWindow</a></li>
                <li><a href="#method-config-effectivenbatch">Config.EffectiveNBatch</a></li>
                <li><a href="#method-config-effectivenubatch">Config.EffectiveNUBatch</a></li>
                <li><a href="#method-config-expertlayersongpu">Config.ExpertLayersOnGPU</a></li>
                <li><a href="#method-config-flashattention">Config.FlashAttention</a></li>
                <li><a href="#method-config-imcsessioncapacity">Config.IMCSessionCapacity</a></li>
                <li><a href="#method-config-incrementalcache">Config.IncrementalCache</a></li>
                <li><a href="#method-config-insecurelogging">Config.InsecureLogging</a></li>
                <li><a href="#method-config-maingpu">Config.MainGPU</a></li>
                <li><a href="#method-config-ngpulayers">Config.NGpuLayers</a></li>
                <li><a href="#method-config-nseqmax">Config.NSeqMax</a></li>
                <li><a href="#method-config-nthreads">Config.NThreads</a></li>
                <li><a href="#method-config-nthreadsbatch">Config.NThreadsBatch</a></li>
                <li><a href="#method-config-opoffloadminbatch">Config.OpOffloadMinBatch</a></li>
                <li><a href="#method-config-prefillbatchsize">Config.PrefillBatchSize</a></li>
                <li><a href="#method-config-queuedepth">Config.QueueDepth</a></li>
                <li><a href="#method-config-ropefreqbase">Config.RopeFreqBase</a></li>
                <li><a href="#method-config-ropefreqscale">Config.RopeFreqScale</a></li>
                <li><a href="#method-config-swafull">Config.SWAFull</a></li>
                <li><a href="#method-config-speculationmode">Config.SpeculationMode</a></li>
                <li><a href="#method-config-string">Config.String</a></li>
                <li><a href="#method-config-yarnattnfactor">Config.YarnAttnFactor</a></li>
                <li><a href="#method-config-yarnbetafast">Config.YarnBetaFast</a></li>
                <li><a href="#method-config-yarnbetaslow">Config.YarnBetaSlow</a></li>
                <li><a href="#method-config-yarnextfactor">Config.YarnExtFactor</a></li>
                <li><a href="#method-config-yarnorigctx">Config.YarnOrigCtx</a></li>
                <li><a href="#method-d-clone">D.Clone</a></li>
                <li><a href="#method-d-messages">D.Messages</a></li>
                <li><a href="#method-d-shallowclone">D.ShallowClone</a></li>
                <li><a href="#method-d-string">D.String</a></li>
                <li><a href="#method-draftmodelconfig-isseparate">DraftModelConfig.IsSeparate</a></li>
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
                <li><a href="#method-loadmode-marshaljson">LoadMode.MarshalJSON</a></li>
                <li><a href="#method-loadmode-marshalyaml">LoadMode.MarshalYAML</a></li>
                <li><a href="#method-loadmode-string">LoadMode.String</a></li>
                <li><a href="#method-loadmode-toyzmatype">LoadMode.ToYZMAType</a></li>
                <li><a href="#method-loadmode-unmarshaljson">LoadMode.UnmarshalJSON</a></li>
                <li><a href="#method-loadmode-unmarshalyaml">LoadMode.UnmarshalYAML</a></li>
                <li><a href="#method-moeconfig-keepexpertsongpufortopnlayers">MoEConfig.KeepExpertsOnGPUForTopNLayers</a></li>
                <li><a href="#method-moemode-equal">MoEMode.Equal</a></li>
                <li><a href="#method-moemode-iszero">MoEMode.IsZero</a></li>
                <li><a href="#method-moemode-marshaltext">MoEMode.MarshalText</a></li>
                <li><a href="#method-moemode-string">MoEMode.String</a></li>
                <li><a href="#method-moemode-unmarshaltext">MoEMode.UnmarshalText</a></li>
                <li><a href="#method-model-batchenginesnapshot">Model.BatchEngineSnapshot</a></li>
                <li><a href="#method-model-chat">Model.Chat</a></li>
                <li><a href="#method-model-chatstreaming">Model.ChatStreaming</a></li>
                <li><a href="#method-model-config">Model.Config</a></li>
                <li><a href="#method-model-embeddings">Model.Embeddings</a></li>
                <li><a href="#method-model-imcsessions">Model.IMCSessions</a></li>
                <li><a href="#method-model-imcsystemcaches">Model.IMCSystemCaches</a></li>
                <li><a href="#method-model-modelinfo">Model.ModelInfo</a></li>
                <li><a href="#method-model-rerank">Model.Rerank</a></li>
                <li><a href="#method-model-tokenize">Model.Tokenize</a></li>
                <li><a href="#method-model-unload">Model.Unload</a></li>
                <li><a href="#method-modelinfo-string">ModelInfo.String</a></li>
                <li><a href="#method-modeltype-string">ModelType.String</a></li>
                <li><a href="#method-params-string">Params.String</a></li>
                <li><a href="#method-responsemessage-marshaljson">ResponseMessage.MarshalJSON</a></li>
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
                <li><a href="#const-speculationauto">SpeculationAuto</a></li>
                <li><a href="#const-defaultprefillbatchsize">DefaultPrefillBatchSize</a></li>
                <li><a href="#const-defaultqueuedepth">DefaultQueueDepth</a></li>
                <li><a href="#const-expertsallongpu">ExpertsAllOnGPU</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-errfileinputsunsupported">ErrFileInputsUnsupported</a></li>
                <li><a href="#var-errinvalidrequest">ErrInvalidRequest</a></li>
                <li><a href="#var-errmessagesinvalid">ErrMessagesInvalid</a></li>
                <li><a href="#var-errmessagesmissing">ErrMessagesMissing</a></li>
                <li><a href="#var-moemodeauto">MoEModeAuto</a></li>
                <li><a href="#var-moemodecustom">MoEModeCustom</a></li>
                <li><a href="#var-moemodeexpertscpu">MoEModeExpertsCPU</a></li>
                <li><a href="#var-moemodeexpertsgpu">MoEModeExpertsGPU</a></li>
                <li><a href="#var-moemodekeeptopn">MoEModeKeepTopN</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
