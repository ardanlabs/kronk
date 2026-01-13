export default function DocsSDKModel() {
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
                <code>func AddParams(p Params, d D)</code>
              </pre>
              <p className="doc-description">AddParams can be used to add the configured parameters to the specified document.</p>
            </div>

            <div className="doc-section" id="func-checkmodel">
              <h4>CheckModel</h4>
              <pre className="code-block">
                <code>func CheckModel(modelFile string, checkSHA bool) error</code>
              </pre>
              <p className="doc-description">CheckModel is check if the downloaded model is valid based on it's sha file. If no sha file exists, this check will return with no error.</p>
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
                <code>func NewModel(ctx context.Context, tmplRetriever TemplateRetriever, cfg Config) (*Model, error)</code>
              </pre>
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
	ID      string   \`json:"id"\`
	Object  string   \`json:"object"\`
	Created int64    \`json:"created"\`
	Model   string   \`json:"model"\`
	Choice  []Choice \`json:"choices"\`
	Usage   Usage    \`json:"usage"\`
	Prompt  string   \`json:"prompt"\`
}`}</code>
              </pre>
              <p className="doc-description">ChatResponse represents output for inference models.</p>
            </div>

            <div className="doc-section" id="type-choice">
              <h4>Choice</h4>
              <pre className="code-block">
                <code>{`type Choice struct {
	Index        int             \`json:"index"\`
	Message      ResponseMessage \`json:"message,omitempty"\`
	Delta        ResponseMessage \`json:"delta,omitempty"\`
	FinishReason string          \`json:"finish_reason"\`
}`}</code>
              </pre>
              <p className="doc-description">Choice represents a single choice in a response.</p>
            </div>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	Log                  Logger
	ModelFiles           []string
	ProjFile             string
	JinjaFile            string
	Device               string
	ContextWindow        int
	NBatch               int
	NUBatch              int
	NThreads             int
	NThreadsBatch        int
	CacheTypeK           GGMLType
	CacheTypeV           GGMLType
	FlashAttention       FlashAttentionType
	UseDirectIO          bool
	IgnoreIntegrityCheck bool
	NSeqMax              int
	OffloadKQV           *bool
	OpOffload            *bool
	NGpuLayers           *int32
	SplitMode            SplitMode
}`}</code>
              </pre>
              <p className="doc-description">Config represents model level configuration. These values if configured incorrectly can cause the system to panic. The defaults are used when these values are set to 0. ModelInstances is the number of instances of the model to create. Unless you have more than 1 GPU, the recommended number of instances is 1. ModelFiles is the path to the model files. This is mandatory to provide. ProjFiles is the path to the projection files. This is mandatory for media based models like vision and audio. JinjaFile is the path to the jinja file. This is not required and can be used if you want to override the templated provided by the model metadata. Device is the device to use for the model. If not set, the default device will be used. To see what devices are available, run the following command which will be found where you installed llama.cpp. $ llama-bench --list-devices ContextWindow (often referred to as context length) is the maximum number of tokens that a large language model can process and consider at one time when generating a response. It defines the model's effective "memory" for a single conversation or text generation task. When set to 0, the default value is 4096. NBatch is the logical batch size or the maximum number of tokens that can be in a single forward pass through the model at any given time. It defines the maximum capacity of the processing batch. If you are processing a very long prompt or multiple prompts simultaneously, the total number of tokens processed in one go will not exceed NBatch. Increasing n_batch can improve performance (throughput) if your hardware can handle it, as it better utilizes parallel computation. However, a very high n_batch can lead to out-of-memory errors on systems with limited VRAM. When set to 0, the default value is 2048. NUBatch is the physical batch size or the maximum number of tokens processed together during the initial prompt processing phase (also called "prompt ingestion") to populate the KV cache. It specifically optimizes the initial loading of prompt tokens into the KV cache. If a prompt is longer than NUBatch, it will be broken down and processed in chunks of n_ubatch tokens sequentially. This parameter is crucial for tuning performance on specific hardware (especially GPUs) because different values might yield better prompt processing times depending on the memory architecture. When set to 0, the default value is 512. NThreads is the number of threads to use for generation. When set to 0, the default llama.cpp value is used. NThreadsBatch is the number of threads to use for batch processing. When set to 0, the default llama.cpp value is used. CacheTypeK is the data type for the K (key) cache. This controls the precision of the key vectors in the KV cache. Lower precision types (like Q8_0 or Q4_0) reduce memory usage but may slightly affect quality. When set to GGMLTypeAuto or left as zero value, the default llama.cpp value (F16) is used. CacheTypeV is the data type for the V (value) cache. This controls the precision of the value vectors in the KV cache. When set to GGMLTypeAuto or left as zero value, the default llama.cpp value (F16) is used. FlashAttention controls Flash Attention mode. Flash Attention reduces memory usage and speeds up attention computation, especially for large context windows. When left as zero value, FlashAttentionEnabled is used (default on). Set to FlashAttentionDisabled to disable, or FlashAttentionAuto to let llama.cpp decide. IgnoreIntegrityCheck is a boolean that determines if the system should ignore a model integrity check before trying to use it. NSeqMax is the maximum number of sequences that can be processed in parallel. This is useful for batched inference where multiple prompts are processed simultaneously. When set to 0, the default llama.cpp value is used. OffloadKQV controls whether the KV cache is offloaded to the GPU. When nil or true, the KV cache is stored on the GPU (default behavior). Set to false to keep the KV cache on the CPU, which reduces VRAM usage but may slow inference. OpOffload controls whether host tensor operations are offloaded to the device (GPU). When nil or true, operations are offloaded (default behavior). Set to false to keep operations on the CPU. NGpuLayers is the number of model layers to offload to the GPU. When set to 0, all layers are offloaded (default). Set to -1 to keep all layers on CPU. Any positive value specifies the exact number of layers to offload. SplitMode controls how the model is split across multiple GPUs: - SplitModeNone (0): single GPU - SplitModeLayer (1): split layers and KV across GPUs - SplitModeRow (2): split layers and KV across GPUs with tensor parallelism (recommended for MoE models like Qwen3-MoE, Mixtral, DeepSeek) When not set, defaults to SplitModeRow for optimal MoE performance.</p>
            </div>

            <div className="doc-section" id="type-d">
              <h4>D</h4>
              <pre className="code-block">
                <code>{`type D map[string]any`}</code>
              </pre>
              <p className="doc-description">D represents a generic docment of fields and values.</p>
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
                <code>{`type Logger func(ctx context.Context, msg string, args ...any)`}</code>
              </pre>
              <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
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
	HasEncoder    bool
	HasDecoder    bool
	IsRecurrent   bool
	IsHybrid      bool
	IsGPTModel    bool
	IsEmbedModel  bool
	Metadata      map[string]string
	TemplateFile  string
	Template      Template
}`}</code>
              </pre>
              <p className="doc-description">ModelInfo represents the model's card information.</p>
            </div>

            <div className="doc-section" id="type-params">
              <h4>Params</h4>
              <pre className="code-block">
                <code>{`type Params struct {
	Temperature     float32 \`json:"temperature"\`
	TopK            int32   \`json:"top_k"\`
	TopP            float32 \`json:"top_p"\`
	MinP            float32 \`json:"min_p"\`
	MaxTokens       int     \`json:"max_tokens"\`
	RepeatPenalty   float32 \`json:"repeat_penalty"\`
	RepeatLastN     int32   \`json:"repeat_last_n"\`
	Thinking        string  \`json:"enable_thinking"\`
	ReasoningEffort string  \`json:"reasoning_effort"\`
	ReturnPrompt    bool    \`json:"return_prompt"\`
}`}</code>
              </pre>
              <p className="doc-description">Params represents the different options when using a model. The defaults are used when these values are set to 0. Temperature controls the randomness of the output. It rescales the probability distribution of possible next tokens. When set to 0, the default value is 0.7. TopK limits the pool of possible next tokens to the K number of most probable tokens. If a model predicts 10,000 possible next tokens, setting Top-K to 50 means only the 50 tokens with the highest probabilities are considered for selection (after temperature scaling). The rest are ignored. When set to 0, the default value is 40. TopP, also known as nucleus sampling, works differently than Top-K by selecting a dynamic pool of tokens whose cumulative probability exceeds a threshold P. Instead of a fixed number of tokens (K), it selects the minimum number of most probable tokens required to reach the cumulative probability P. When set to 0, the default value is 0.9. MinP, is a dynamic sampling threshold that helps balance the coherence (quality) and diversity (creativity) of the generated text. When set to 0, the default value is 0.0. These parameters (TopK, TopP, Temperature) are typically used together. The sampling process usually applies temperature first, then filters the token list using Top-K, and finally filters it again using Top-P before selecting the next token randomly from the remaining pool based on their (now adjusted) probabilities. MaxTokens defines the maximum number of output tokens to generate for a single response. When set to 0, the default value is 512. EnableThinking determines if the model should think or not. It is used for most non-GPT models. It accepts 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False. When set to an empty string, the default value is "true". ReasoningEffort is a string that specifies the level of reasoning effort to use for GPT models. RepeatPenalty applies a penalty to tokens that have already appeared in the output, reducing repetitive text. A value of 1.0 means no penalty. Values above 1.0 reduce repetition (e.g., 1.1 is a mild penalty, 1.5 is strong). When set to 0, the default value is 1.1. RepeatLastN specifies how many recent tokens to consider when applying the repetition penalty. A larger value considers more context but may be slower. When set to 0, the default value is 64. ReturnPrompt determines whether to include the prompt in the final response. When set to true, the prompt will be included. Default is false.</p>
            </div>

            <div className="doc-section" id="type-responsemessage">
              <h4>ResponseMessage</h4>
              <pre className="code-block">
                <code>{`type ResponseMessage struct {
	Role      string             \`json:"role"\`
	Content   string             \`json:"content"\`
	Reasoning string             \`json:"reasoning"\`
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

            <div className="doc-section" id="type-splitmode">
              <h4>SplitMode</h4>
              <pre className="code-block">
                <code>{`type SplitMode int32`}</code>
              </pre>
              <p className="doc-description">SplitMode controls how the model is split across multiple GPUs. This is particularly important for Mixture of Experts (MoE) models.</p>
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

            <div className="doc-section" id="type-templateretriever">
              <h4>TemplateRetriever</h4>
              <pre className="code-block">
                <code>{`type TemplateRetriever interface {
	Retrieve(modelID string) (Template, error)
}`}</code>
              </pre>
              <p className="doc-description">TemplateRetriever returns a configured template for a model.</p>
            </div>

            <div className="doc-section" id="type-toolcallarguments">
              <h4>ToolCallArguments</h4>
              <pre className="code-block">
                <code>{`type ToolCallArguments map[string]any`}</code>
              </pre>
              <p className="doc-description">ToolCallArguments represents tool call arguments that marshal to a JSON string per OpenAI API spec, but can unmarshal from either a string or object.</p>
            </div>

            <div className="doc-section" id="type-usage">
              <h4>Usage</h4>
              <pre className="code-block">
                <code>{`type Usage struct {
	PromptTokens     int     \`json:"prompt_tokens"\`
	ReasoningTokens  int     \`json:"reasoning_tokens"\`
	CompletionTokens int     \`json:"completion_tokens"\`
	OutputTokens     int     \`json:"output_tokens"\`
	TotalTokens      int     \`json:"total_tokens"\`
	TokensPerSecond  float64 \`json:"tokens_per_second"\`
}`}</code>
              </pre>
              <p className="doc-description">Usage provides details usage information for the request.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-d-clone">
              <h4>D.Clone</h4>
              <pre className="code-block">
                <code>func (d D) Clone() D</code>
              </pre>
              <p className="doc-description">Clone creates a shallow copy of the document. This is useful when you need to modify the document without affecting the original.</p>
            </div>

            <div className="doc-section" id="method-d-logsafe">
              <h4>D.LogSafe</h4>
              <pre className="code-block">
                <code>func (d D) LogSafe() D</code>
              </pre>
              <p className="doc-description">LogSafe returns a copy of the document containing only fields that are safe to log. This excludes sensitive fields like messages and input which may contain private user data.</p>
            </div>

            <div className="doc-section" id="method-flashattentiontype-unmarshalyaml">
              <h4>FlashAttentionType.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (t *FlashAttentionType) UnmarshalYAML(unmarshal func(interface&#123;&#125;) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values.</p>
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

            <div className="doc-section" id="method-ggmltype-unmarshalyaml">
              <h4>GGMLType.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (t *GGMLType) UnmarshalYAML(unmarshal func(interface&#123;&#125;) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values like "f16".</p>
            </div>

            <div className="doc-section" id="method-model-chat">
              <h4>Model.Chat</h4>
              <pre className="code-block">
                <code>func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error)</code>
              </pre>
              <p className="doc-description">Chat performs a chat request and returns the final response.</p>
            </div>

            <div className="doc-section" id="method-model-chatstreaming">
              <h4>Model.ChatStreaming</h4>
              <pre className="code-block">
                <code>func (m *Model) ChatStreaming(ctx context.Context, d D) &lt;-chan ChatResponse</code>
              </pre>
              <p className="doc-description">ChatStreaming performs a chat request and streams the response.</p>
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
              <p className="doc-description">Embeddings performs an embedding request and returns the final response. Supported options in d: - input (string): the text to embed (required) - truncate (bool): if true, truncate input to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models)</p>
            </div>

            <div className="doc-section" id="method-model-modelinfo">
              <h4>Model.ModelInfo</h4>
              <pre className="code-block">
                <code>func (m *Model) ModelInfo() ModelInfo</code>
              </pre>
            </div>

            <div className="doc-section" id="method-model-unload">
              <h4>Model.Unload</h4>
              <pre className="code-block">
                <code>func (m *Model) Unload(ctx context.Context) error</code>
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

            <div className="doc-section" id="method-splitmode-unmarshalyaml">
              <h4>SplitMode.UnmarshalYAML</h4>
              <pre className="code-block">
                <code>func (s *SplitMode) UnmarshalYAML(unmarshal func(interface&#123;&#125;) error) error</code>
              </pre>
              <p className="doc-description">UnmarshalYAML implements yaml.Unmarshaler to parse string values.</p>
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

            <div className="doc-section" id="const-objectchatunknown">
              <h4>ObjectChatUnknown</h4>
              <pre className="code-block">
                <code>{`const (
	ObjectChatUnknown = "chat.unknown"
	ObjectChatText    = "chat.completion.chunk"
	ObjectChatMedia   = "chat.media"
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
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-addparams">AddParams</a></li>
                <li><a href="#func-checkmodel">CheckModel</a></li>
                <li><a href="#func-parseggmltype">ParseGGMLType</a></li>
                <li><a href="#func-newmodel">NewModel</a></li>
                <li><a href="#func-parsesplitmode">ParseSplitMode</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-chatresponse">ChatResponse</a></li>
                <li><a href="#type-choice">Choice</a></li>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-d">D</a></li>
                <li><a href="#type-embeddata">EmbedData</a></li>
                <li><a href="#type-embedreponse">EmbedReponse</a></li>
                <li><a href="#type-embedusage">EmbedUsage</a></li>
                <li><a href="#type-flashattentiontype">FlashAttentionType</a></li>
                <li><a href="#type-ggmltype">GGMLType</a></li>
                <li><a href="#type-logger">Logger</a></li>
                <li><a href="#type-model">Model</a></li>
                <li><a href="#type-modelinfo">ModelInfo</a></li>
                <li><a href="#type-params">Params</a></li>
                <li><a href="#type-responsemessage">ResponseMessage</a></li>
                <li><a href="#type-responsetoolcall">ResponseToolCall</a></li>
                <li><a href="#type-responsetoolcallfunction">ResponseToolCallFunction</a></li>
                <li><a href="#type-splitmode">SplitMode</a></li>
                <li><a href="#type-template">Template</a></li>
                <li><a href="#type-templateretriever">TemplateRetriever</a></li>
                <li><a href="#type-toolcallarguments">ToolCallArguments</a></li>
                <li><a href="#type-usage">Usage</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-d-clone">D.Clone</a></li>
                <li><a href="#method-d-logsafe">D.LogSafe</a></li>
                <li><a href="#method-flashattentiontype-unmarshalyaml">FlashAttentionType.UnmarshalYAML</a></li>
                <li><a href="#method-ggmltype-string">GGMLType.String</a></li>
                <li><a href="#method-ggmltype-toyzmatype">GGMLType.ToYZMAType</a></li>
                <li><a href="#method-ggmltype-unmarshalyaml">GGMLType.UnmarshalYAML</a></li>
                <li><a href="#method-model-chat">Model.Chat</a></li>
                <li><a href="#method-model-chatstreaming">Model.ChatStreaming</a></li>
                <li><a href="#method-model-config">Model.Config</a></li>
                <li><a href="#method-model-embeddings">Model.Embeddings</a></li>
                <li><a href="#method-model-modelinfo">Model.ModelInfo</a></li>
                <li><a href="#method-model-unload">Model.Unload</a></li>
                <li><a href="#method-splitmode-string">SplitMode.String</a></li>
                <li><a href="#method-splitmode-toyzmatype">SplitMode.ToYZMAType</a></li>
                <li><a href="#method-splitmode-unmarshalyaml">SplitMode.UnmarshalYAML</a></li>
                <li><a href="#method-toolcallarguments-marshaljson">ToolCallArguments.MarshalJSON</a></li>
                <li><a href="#method-toolcallarguments-unmarshaljson">ToolCallArguments.UnmarshalJSON</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#constants" className="doc-index-header">Constants</a>
              <ul>
                <li><a href="#const-objectchatunknown">ObjectChatUnknown</a></li>
                <li><a href="#const-roleuser">RoleUser</a></li>
                <li><a href="#const-finishreasonstop">FinishReasonStop</a></li>
                <li><a href="#const-thinkingenabled">ThinkingEnabled</a></li>
                <li><a href="#const-reasoningeffortnone">ReasoningEffortNone</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
