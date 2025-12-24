export default function DocsSDKModel() {
  return (
    <div>
      <div className="page-header">
        <h2>Model Package</h2>
        <p>Low-level API for working with models</p>
      </div>

      <div className="card">
        <h3>Import</h3>
        <pre className="code-block">
          <code>import "github.com/ardanlabs/kronk/sdk/kronk/model"</code>
        </pre>
      </div>

      <div className="card">
        <h3>Constants</h3>

        <div className="doc-section">
          <h4>Object Types</h4>
          <pre className="code-block">
            <code>{`const (
    ObjectChatUnknown = "chat.unknown"
    ObjectChatText    = "chat.completion.chunk"
    ObjectChatMedia   = "chat.media"
)`}</code>
          </pre>
          <p className="doc-description">Objects represent the different types of data that is being processed.</p>
        </div>

        <div className="doc-section">
          <h4>Roles</h4>
          <pre className="code-block">
            <code>{`const (
    RoleAssistant = "assistant"
)`}</code>
          </pre>
          <p className="doc-description">Roles represent the different roles that can be used in a chat.</p>
        </div>

        <div className="doc-section">
          <h4>Finish Reasons</h4>
          <pre className="code-block">
            <code>{`const (
    FinishReasonStop  = "stop"
    FinishReasonTool  = "tool_calls"
    FinishReasonError = "error"
)`}</code>
          </pre>
          <p className="doc-description">FinishReasons represent the different reasons a response can be finished.</p>
        </div>

        <div className="doc-section">
          <h4>Thinking Constants</h4>
          <pre className="code-block">
            <code>{`const (
    ThinkingEnabled  = "true"   // The model will perform thinking (default)
    ThinkingDisabled = "false"  // The model will not perform thinking
)`}</code>
          </pre>
        </div>

        <div className="doc-section">
          <h4>Reasoning Effort Constants</h4>
          <pre className="code-block">
            <code>{`const (
    ReasoningEffortNone    = "none"     // No reasoning, fastest
    ReasoningEffortMinimal = "minimal"  // Very low reasoning
    ReasoningEffortLow     = "low"      // Light reasoning
    ReasoningEffortMedium  = "medium"   // Default, balanced
    ReasoningEffortHigh    = "high"     // Extensive reasoning
)`}</code>
          </pre>
          <p className="doc-description">ReasoningEffort specifies the level of reasoning effort for GPT models.</p>
        </div>
      </div>

      <div className="card">
        <h3>Types</h3>

        <div className="doc-section">
          <h4>Config</h4>
          <pre className="code-block">
            <code>{`type Config struct {
    Log           Logger
    ModelFile     string
    ProjFile      string
    JinjaFile     string
    Device        string
    ContextWindow int
    NBatch        int
    NUBatch       int
    NThreads      int
    NThreadsBatch int
}`}</code>
          </pre>
          <p className="doc-description">
            Config represents model level configuration. These values if configured incorrectly can cause
            the system to panic.
          </p>
          <ul className="doc-list">
            <li>
              <strong>ModelFile</strong> - Path to the model file (mandatory)
            </li>
            <li>
              <strong>ProjFile</strong> - Path to the projection file (required for vision/audio models)
            </li>
            <li>
              <strong>JinjaFile</strong> - Optional path to override the model's template
            </li>
            <li>
              <strong>Device</strong> - Device to use (run llama-bench --list-devices to see available)
            </li>
            <li>
              <strong>ContextWindow</strong> - Maximum tokens the model can process (default: 4096)
            </li>
            <li>
              <strong>NBatch</strong> - Logical batch size for forward pass (default: 2048)
            </li>
            <li>
              <strong>NUBatch</strong> - Physical batch size for prompt processing (default: 512)
            </li>
            <li>
              <strong>NThreads</strong> - Number of threads for generation
            </li>
            <li>
              <strong>NThreadsBatch</strong> - Number of threads for batch processing
            </li>
          </ul>
        </div>

        <div className="doc-section">
          <h4>Logger</h4>
          <pre className="code-block">
            <code>type Logger func(ctx context.Context, msg string, args ...any)</code>
          </pre>
          <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
        </div>

        <div className="doc-section">
          <h4>Model</h4>
          <pre className="code-block">
            <code>{`type Model struct {
    // contains filtered or unexported fields
}`}</code>
          </pre>
          <p className="doc-description">
            Model represents a model and provides a low-level API for working with it.
          </p>
        </div>

        <div className="doc-section">
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

        <div className="doc-section">
          <h4>D</h4>
          <pre className="code-block">
            <code>type D map[string]any</code>
          </pre>
          <p className="doc-description">D represents a generic document of fields and values.</p>
        </div>

        <div className="doc-section">
          <h4>Params</h4>
          <pre className="code-block">
            <code>{`type Params struct {
    Temperature     float32  // Randomness of output (default: 0.8)
    TopK            int32    // Limit to K most probable tokens (default: 40)
    TopP            float32  // Nucleus sampling threshold (default: 0.9)
    MinP            float32  // Dynamic sampling threshold (default: 0.0)
    MaxTokens       int      // Maximum output tokens (default: 1024)
    Thinking        string   // Enable thinking mode
    ReasoningEffort string   // Reasoning level for GPT models
}`}</code>
          </pre>
          <p className="doc-description">
            Params represents the different options when using a model. Temperature is applied first, then
            Top-K filters the token list, then Top-P filters again before selection.
          </p>
        </div>

        <div className="doc-section">
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

        <div className="doc-section">
          <h4>Choice</h4>
          <pre className="code-block">
            <code>{`type Choice struct {
    Index        int             \`json:"index"\`
    Delta        ResponseMessage \`json:"delta"\`
    FinishReason string          \`json:"finish_reason"\`
}`}</code>
          </pre>
          <p className="doc-description">Choice represents a single choice in a response.</p>
        </div>

        <div className="doc-section">
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

        <div className="doc-section">
          <h4>ResponseToolCall</h4>
          <pre className="code-block">
            <code>{`type ResponseToolCall struct {
    ID        string         \`json:"id"\`
    Name      string         \`json:"name"\`
    Arguments map[string]any \`json:"arguments"\`
    Status    int            \`json:"status"\`
    Raw       string         \`json:"raw"\`
    Error     string         \`json:"error"\`
}`}</code>
          </pre>
        </div>

        <div className="doc-section">
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
          <p className="doc-description">Usage provides detailed usage information for the request.</p>
        </div>

        <div className="doc-section">
          <h4>EmbedReponse</h4>
          <pre className="code-block">
            <code>{`type EmbedReponse struct {
    Object  string      \`json:"object"\`
    Created int64       \`json:"created"\`
    Model   string      \`json:"model"\`
    Data    []EmbedData \`json:"data"\`
}`}</code>
          </pre>
          <p className="doc-description">EmbedReponse represents the output for an embedding call.</p>
        </div>

        <div className="doc-section">
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

        <div className="doc-section">
          <h4>Template</h4>
          <pre className="code-block">
            <code>{`type Template struct {
    FileName string
    Script   string
}`}</code>
          </pre>
          <p className="doc-description">Template provides the template file name and script content.</p>
        </div>

        <div className="doc-section">
          <h4>TemplateRetriever</h4>
          <pre className="code-block">
            <code>{`type TemplateRetriever interface {
    Retrieve(modelID string) (Template, error)
}`}</code>
          </pre>
          <p className="doc-description">TemplateRetriever returns a configured template for a model.</p>
        </div>
      </div>

      <div className="card">
        <h3>Functions</h3>

        <div className="doc-section">
          <h4>NewModel</h4>
          <pre className="code-block">
            <code>func NewModel(tmlpRetriever TemplateRetriever, cfg Config) (*Model, error)</code>
          </pre>
          <p className="doc-description">
            NewModel creates a new Model instance with the specified template retriever and configuration.
          </p>
        </div>

        <div className="doc-section">
          <h4>TextMessage</h4>
          <pre className="code-block">
            <code>func TextMessage(role string, content string) D</code>
          </pre>
          <p className="doc-description">TextMessage creates a new text message document.</p>
        </div>

        <div className="doc-section">
          <h4>MediaMessage</h4>
          <pre className="code-block">
            <code>func MediaMessage(text string, media []byte) []D</code>
          </pre>
          <p className="doc-description">MediaMessage creates a new media message document.</p>
        </div>

        <div className="doc-section">
          <h4>DocumentArray</h4>
          <pre className="code-block">
            <code>func DocumentArray(doc ...D) []D</code>
          </pre>
          <p className="doc-description">DocumentArray creates a new document array from the provided documents.</p>
        </div>

        <div className="doc-section">
          <h4>MapToModelD</h4>
          <pre className="code-block">
            <code>func MapToModelD(m map[string]any) D</code>
          </pre>
          <p className="doc-description">MapToModelD converts a map[string]any to a D.</p>
        </div>

        <div className="doc-section">
          <h4>AddParams</h4>
          <pre className="code-block">
            <code>func AddParams(p Params, d D)</code>
          </pre>
          <p className="doc-description">AddParams adds the configured parameters to the specified document.</p>
        </div>

        <div className="doc-section">
          <h4>ChatResponseErr</h4>
          <pre className="code-block">
            <code>func ChatResponseErr(id string, object string, model string, index int, prompt string, err error, u Usage) ChatResponse</code>
          </pre>
          <p className="doc-description">ChatResponseErr creates a ChatResponse representing an error.</p>
        </div>
      </div>

      <div className="card">
        <h3>Methods</h3>

        <div className="doc-section">
          <h4>Model.Config</h4>
          <pre className="code-block">
            <code>func (m *Model) Config() Config</code>
          </pre>
          <p className="doc-description">Config returns the model's configuration.</p>
        </div>

        <div className="doc-section">
          <h4>Model.ModelInfo</h4>
          <pre className="code-block">
            <code>func (m *Model) ModelInfo() ModelInfo</code>
          </pre>
          <p className="doc-description">ModelInfo returns the model information.</p>
        </div>

        <div className="doc-section">
          <h4>Model.Unload</h4>
          <pre className="code-block">
            <code>func (m *Model) Unload(ctx context.Context) error</code>
          </pre>
          <p className="doc-description">Unload closes down the model and frees resources.</p>
        </div>

        <div className="doc-section">
          <h4>Model.Chat</h4>
          <pre className="code-block">
            <code>func (m *Model) Chat(ctx context.Context, d D) (ChatResponse, error)</code>
          </pre>
          <p className="doc-description">Chat performs a chat request and returns the final response.</p>
        </div>

        <div className="doc-section">
          <h4>Model.ChatStreaming</h4>
          <pre className="code-block">
            <code>func (m *Model) ChatStreaming(ctx context.Context, d D) &lt;-chan ChatResponse</code>
          </pre>
          <p className="doc-description">ChatStreaming performs a chat request and streams the response.</p>
        </div>

        <div className="doc-section">
          <h4>Model.Embeddings</h4>
          <pre className="code-block">
            <code>func (m *Model) Embeddings(ctx context.Context, input string) (EmbedReponse, error)</code>
          </pre>
          <p className="doc-description">
            Embeddings performs an embedding request and returns the response. Returns an error if the
            model doesn't support embedding.
          </p>
        </div>
      </div>
    </div>
  );
}
