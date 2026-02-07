export default function DocsSDKKronk() {
  return (
    <div>
      <div className="page-header">
        <h2>Kronk Package</h2>
        <p>Package kronk provides support for working with models using llama.cpp via yzma.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/kronk"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-init">
              <h4>Init</h4>
              <pre className="code-block">
                <code>func Init(opts ...InitOption) error</code>
              </pre>
              <p className="doc-description">Init initializes the Kronk backend support.</p>
            </div>

            <div className="doc-section" id="func-setfmtloggertraceid">
              <h4>SetFmtLoggerTraceID</h4>
              <pre className="code-block">
                <code>func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context</code>
              </pre>
              <p className="doc-description">SetFmtLoggerTraceID allows you to set a trace id in the content that can be part of the output of the FmtLogger.</p>
            </div>

            <div className="doc-section" id="func-new">
              <h4>New</h4>
              <pre className="code-block">
                <code>func New(cfg model.Config, opts ...Option) (*Kronk, error)</code>
              </pre>
              <p className="doc-description">New provides the ability to use models in a concurrently safe way.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-incompletedetail">
              <h4>IncompleteDetail</h4>
              <pre className="code-block">
                <code>{`type IncompleteDetail struct {
	Reason string \`json:"reason"\`
}`}</code>
              </pre>
              <p className="doc-description">IncompleteDetail provides details about why a response is incomplete.</p>
            </div>

            <div className="doc-section" id="type-initoption">
              <h4>InitOption</h4>
              <pre className="code-block">
                <code>{`type InitOption func(*initOptions)`}</code>
              </pre>
              <p className="doc-description">InitOption represents options for configuring Init.</p>
            </div>

            <div className="doc-section" id="type-inputtokensdetails">
              <h4>InputTokensDetails</h4>
              <pre className="code-block">
                <code>{`type InputTokensDetails struct {
	CachedTokens int \`json:"cached_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">InputTokensDetails provides breakdown of input tokens.</p>
            </div>

            <div className="doc-section" id="type-kronk">
              <h4>Kronk</h4>
              <pre className="code-block">
                <code>{`type Kronk struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Kronk provides a concurrently safe api for using llama.cpp to access models.</p>
            </div>

            <div className="doc-section" id="type-loglevel">
              <h4>LogLevel</h4>
              <pre className="code-block">
                <code>{`type LogLevel int`}</code>
              </pre>
              <p className="doc-description">LogLevel represents the logging level.</p>
            </div>

            <div className="doc-section" id="type-logger">
              <h4>Logger</h4>
              <pre className="code-block">
                <code>{`type Logger func(ctx context.Context, msg string, args ...any)`}</code>
              </pre>
              <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
            </div>

            <div className="doc-section" id="type-option">
              <h4>Option</h4>
              <pre className="code-block">
                <code>{`type Option func(*options)`}</code>
              </pre>
              <p className="doc-description">Option represents options for configuring Kronk.</p>
            </div>

            <div className="doc-section" id="type-outputtokensdetails">
              <h4>OutputTokensDetails</h4>
              <pre className="code-block">
                <code>{`type OutputTokensDetails struct {
	ReasoningTokens int \`json:"reasoning_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">OutputTokensDetails provides breakdown of output tokens.</p>
            </div>

            <div className="doc-section" id="type-responsecontentitem">
              <h4>ResponseContentItem</h4>
              <pre className="code-block">
                <code>{`type ResponseContentItem struct {
	Type        string   \`json:"type"\`
	Text        string   \`json:"text"\`
	Annotations []string \`json:"annotations"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseContentItem represents a content item within an output message.</p>
            </div>

            <div className="doc-section" id="type-responseerror">
              <h4>ResponseError</h4>
              <pre className="code-block">
                <code>{`type ResponseError struct {
	Code    string \`json:"code"\`
	Message string \`json:"message"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseError represents an error in the response.</p>
            </div>

            <div className="doc-section" id="type-responseformattype">
              <h4>ResponseFormatType</h4>
              <pre className="code-block">
                <code>{`type ResponseFormatType struct {
	Type string \`json:"type"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseFormatType specifies the format type.</p>
            </div>

            <div className="doc-section" id="type-responseoutputitem">
              <h4>ResponseOutputItem</h4>
              <pre className="code-block">
                <code>{`type ResponseOutputItem struct {
	Type      string                \`json:"type"\`
	ID        string                \`json:"id"\`
	Status    string                \`json:"status,omitempty"\`
	Role      string                \`json:"role,omitempty"\`
	Content   []ResponseContentItem \`json:"content,omitempty"\`
	CallID    string                \`json:"call_id,omitempty"\`
	Name      string                \`json:"name,omitempty"\`
	Arguments string                \`json:"arguments,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseOutputItem represents an item in the output array. For type="message": ID, Status, Role, Content are used. For type="function_call": ID, Status, CallID, Name, Arguments are used.</p>
            </div>

            <div className="doc-section" id="type-responsereasoning">
              <h4>ResponseReasoning</h4>
              <pre className="code-block">
                <code>{`type ResponseReasoning struct {
	Effort  *string \`json:"effort"\`
	Summary *string \`json:"summary"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseReasoning contains reasoning configuration/output.</p>
            </div>

            <div className="doc-section" id="type-responseresponse">
              <h4>ResponseResponse</h4>
              <pre className="code-block">
                <code>{`type ResponseResponse struct {
	ID               string                 \`json:"id"\`
	Object           string                 \`json:"object"\`
	CreatedAt        int64                  \`json:"created_at"\`
	Status           string                 \`json:"status"\`
	CompletedAt      *int64                 \`json:"completed_at"\`
	Error            *ResponseError         \`json:"error"\`
	IncompleteDetail *IncompleteDetail      \`json:"incomplete_details"\`
	Instructions     *string                \`json:"instructions"\`
	MaxOutputTokens  *int                   \`json:"max_output_tokens"\`
	Model            string                 \`json:"model"\`
	Output           []ResponseOutputItem   \`json:"output"\`
	ParallelToolCall bool                   \`json:"parallel_tool_calls"\`
	PrevResponseID   *string                \`json:"previous_response_id"\`
	Reasoning        ResponseReasoning      \`json:"reasoning"\`
	Store            bool                   \`json:"store"\`
	Temperature      float64                \`json:"temperature"\`
	Text             ResponseTextFormat     \`json:"text"\`
	ToolChoice       string                 \`json:"tool_choice"\`
	Tools            []any                  \`json:"tools"\`
	TopP             float64                \`json:"top_p"\`
	Truncation       string                 \`json:"truncation"\`
	Usage            ResponseUsage          \`json:"usage"\`
	User             *string                \`json:"user"\`
	Metadata         map[string]interface{} \`json:"metadata"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseResponse represents the OpenAI Responses API response format.</p>
            </div>

            <div className="doc-section" id="type-responsestreamevent">
              <h4>ResponseStreamEvent</h4>
              <pre className="code-block">
                <code>{`type ResponseStreamEvent struct {
	Type           string               \`json:"type"\`
	SequenceNumber int                  \`json:"sequence_number"\`
	Response       *ResponseResponse    \`json:"response,omitempty"\`
	OutputIndex    *int                 \`json:"output_index,omitempty"\`
	ContentIndex   *int                 \`json:"content_index,omitempty"\`
	ItemID         string               \`json:"item_id,omitempty"\`
	Item           *ResponseOutputItem  \`json:"item,omitempty"\`
	Part           *ResponseContentItem \`json:"part,omitempty"\`
	Delta          string               \`json:"delta,omitempty"\`
	Text           string               \`json:"text,omitempty"\`
	Arguments      string               \`json:"arguments,omitempty"\`
	Name           string               \`json:"name,omitempty"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseStreamEvent represents a streaming event for the Responses API.</p>
            </div>

            <div className="doc-section" id="type-responsetextformat">
              <h4>ResponseTextFormat</h4>
              <pre className="code-block">
                <code>{`type ResponseTextFormat struct {
	Format ResponseFormatType \`json:"format"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseTextFormat specifies the text format configuration.</p>
            </div>

            <div className="doc-section" id="type-responseusage">
              <h4>ResponseUsage</h4>
              <pre className="code-block">
                <code>{`type ResponseUsage struct {
	InputTokens        int                 \`json:"input_tokens"\`
	InputTokensDetails InputTokensDetails  \`json:"input_tokens_details"\`
	OutputTokens       int                 \`json:"output_tokens"\`
	OutputTokenDetail  OutputTokensDetails \`json:"output_tokens_details"\`
	TotalTokens        int                 \`json:"total_tokens"\`
}`}</code>
              </pre>
              <p className="doc-description">ResponseUsage contains token usage information.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-kronk-activestreams">
              <h4>Kronk.ActiveStreams</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ActiveStreams() int</code>
              </pre>
              <p className="doc-description">ActiveStreams returns the number of active streams.</p>
            </div>

            <div className="doc-section" id="method-kronk-chat">
              <h4>Kronk.Chat</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error)</code>
              </pre>
              <p className="doc-description">Chat provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-chatstreaming">
              <h4>Kronk.ChatStreaming</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (&lt;-chan model.ChatResponse, error)</code>
              </pre>
              <p className="doc-description">ChatStreaming provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-chatstreaminghttp">
              <h4>Kronk.ChatStreamingHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error)</code>
              </pre>
              <p className="doc-description">ChatStreamingHTTP provides http handler support for a chat/completions call. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-embeddings">
              <h4>Kronk.Embeddings</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Embeddings(ctx context.Context, d model.D) (model.EmbedReponse, error)</code>
              </pre>
              <p className="doc-description">Embeddings provides support to interact with an embedding model. Supported options in d: - input (string): the text to embed (required) - truncate (bool): if true, truncate input to fit context window (default: false) - truncate_direction (string): "right" (default) or "left" - dimensions (int): reduce output to first N dimensions (for Matryoshka models) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for embedding extraction). Use NSeqMax &gt; 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request.</p>
            </div>

            <div className="doc-section" id="method-kronk-embeddingshttp">
              <h4>Kronk.EmbeddingsHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error)</code>
              </pre>
              <p className="doc-description">EmbeddingsHTTP provides http handler support for an embeddings call.</p>
            </div>

            <div className="doc-section" id="method-kronk-modelconfig">
              <h4>Kronk.ModelConfig</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ModelConfig() model.Config</code>
              </pre>
              <p className="doc-description">ModelConfig returns a copy of the configuration being used. This may be different from the configuration passed to New() if the model has overridden any of the settings.</p>
            </div>

            <div className="doc-section" id="method-kronk-modelinfo">
              <h4>Kronk.ModelInfo</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ModelInfo() model.ModelInfo</code>
              </pre>
              <p className="doc-description">ModelInfo returns the model information.</p>
            </div>

            <div className="doc-section" id="method-kronk-rerank">
              <h4>Kronk.Rerank</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Rerank(ctx context.Context, d model.D) (model.RerankResponse, error)</code>
              </pre>
              <p className="doc-description">Rerank provides support to interact with a reranker model. Supported options in d: - query (string): the query to rank documents against (required) - documents ([]string): the documents to rank (required) - top_n (int): return only the top N results (optional, default: all) - return_documents (bool): include document text in results (default: false) Each model instance processes calls sequentially (llama.cpp only supports sequence 0 for rerank extraction). Use NSeqMax &gt; 1 to create multiple model instances for concurrent request handling. Batch multiple texts in the input parameter for better performance within a single request.</p>
            </div>

            <div className="doc-section" id="method-kronk-rerankhttp">
              <h4>Kronk.RerankHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) RerankHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.RerankResponse, error)</code>
              </pre>
              <p className="doc-description">RerankHTTP provides http handler support for a rerank call.</p>
            </div>

            <div className="doc-section" id="method-kronk-response">
              <h4>Kronk.Response</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Response(ctx context.Context, d model.D) (ResponseResponse, error)</code>
              </pre>
              <p className="doc-description">Response provides support to interact with an inference model. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-responsestreaming">
              <h4>Kronk.ResponseStreaming</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ResponseStreaming(ctx context.Context, d model.D) (&lt;-chan ResponseStreamEvent, error)</code>
              </pre>
              <p className="doc-description">ResponseStreaming provides streaming support for the Responses API. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-responsestreaminghttp">
              <h4>Kronk.ResponseStreamingHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ResponseStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (ResponseResponse, error)</code>
              </pre>
              <p className="doc-description">ResponseStreamingHTTP provides http handler support for a responses call. For text models, NSeqMax controls parallel sequence processing within a single model instance. For vision/audio models, NSeqMax creates multiple model instances in a pool for concurrent request handling.</p>
            </div>

            <div className="doc-section" id="method-kronk-systeminfo">
              <h4>Kronk.SystemInfo</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) SystemInfo() map[string]string</code>
              </pre>
              <p className="doc-description">SystemInfo returns system information.</p>
            </div>

            <div className="doc-section" id="method-kronk-tokenize">
              <h4>Kronk.Tokenize</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Tokenize(ctx context.Context, d model.D) (model.TokenizeResponse, error)</code>
              </pre>
              <p className="doc-description">Tokenize returns the token count for a text input. Supported options in d: - input (string): the text to tokenize (required) - apply_template (bool): if true, wrap input as a user message and apply the model's chat template before tokenizing (default: false) - add_generation_prompt (bool): when apply_template is true, controls whether the assistant role prefix is appended to the prompt (default: true) When apply_template is true, the returned count includes all template overhead (role markers, separators, generation prompt). This reflects the actual number of tokens that would be fed to the model.</p>
            </div>

            <div className="doc-section" id="method-kronk-tokenizehttp">
              <h4>Kronk.TokenizeHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) TokenizeHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.TokenizeResponse, error)</code>
              </pre>
              <p className="doc-description">TokenizeHTTP provides http handler support for a tokenize call.</p>
            </div>

            <div className="doc-section" id="method-kronk-unload">
              <h4>Kronk.Unload</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Unload(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Unload will close down the loaded model. You should call this only when you are completely done using Kronk.</p>
            </div>

            <div className="doc-section" id="method-loglevel-int">
              <h4>LogLevel.Int</h4>
              <pre className="code-block">
                <code>func (ll LogLevel) Int() int</code>
              </pre>
              <p className="doc-description">Int returns the integer value.</p>
            </div>
          </div>

          <div className="card" id="constants">
            <h3>Constants</h3>

            <div className="doc-section" id="const-version">
              <h4>Version</h4>
              <pre className="code-block">
                <code>{`const Version = "1.17.8"`}</code>
              </pre>
              <p className="doc-description">Version contains the current version of the kronk package.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-discardlogger">
              <h4>DiscardLogger</h4>
              <pre className="code-block">
                <code>{`var DiscardLogger = func(ctx context.Context, msg string, args ...any) {
}`}</code>
              </pre>
              <p className="doc-description">DiscardLogger discards logging.</p>
            </div>

            <div className="doc-section" id="var-fmtlogger">
              <h4>FmtLogger</h4>
              <pre className="code-block">
                <code>{`var FmtLogger = func(ctx context.Context, msg string, args ...any) {
	traceID, ok := ctx.Value(traceIDKey(1)).(string)
	switch ok {
	case true:
		fmt.Printf("traceID: %s: %s:", traceID, msg)
	default:
		fmt.Printf("%s:", msg)
	}

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v[%v]", args[i], args[i+1])
		}
	}

	if len(msg) > 0 && msg[0] != '\\r' {
		fmt.Println()
	}
}`}</code>
              </pre>
              <p className="doc-description">FmtLogger provides a basic logger that writes to stdout.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-init">Init</a></li>
                <li><a href="#func-setfmtloggertraceid">SetFmtLoggerTraceID</a></li>
                <li><a href="#func-new">New</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-incompletedetail">IncompleteDetail</a></li>
                <li><a href="#type-initoption">InitOption</a></li>
                <li><a href="#type-inputtokensdetails">InputTokensDetails</a></li>
                <li><a href="#type-kronk">Kronk</a></li>
                <li><a href="#type-loglevel">LogLevel</a></li>
                <li><a href="#type-logger">Logger</a></li>
                <li><a href="#type-option">Option</a></li>
                <li><a href="#type-outputtokensdetails">OutputTokensDetails</a></li>
                <li><a href="#type-responsecontentitem">ResponseContentItem</a></li>
                <li><a href="#type-responseerror">ResponseError</a></li>
                <li><a href="#type-responseformattype">ResponseFormatType</a></li>
                <li><a href="#type-responseoutputitem">ResponseOutputItem</a></li>
                <li><a href="#type-responsereasoning">ResponseReasoning</a></li>
                <li><a href="#type-responseresponse">ResponseResponse</a></li>
                <li><a href="#type-responsestreamevent">ResponseStreamEvent</a></li>
                <li><a href="#type-responsetextformat">ResponseTextFormat</a></li>
                <li><a href="#type-responseusage">ResponseUsage</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-kronk-activestreams">Kronk.ActiveStreams</a></li>
                <li><a href="#method-kronk-chat">Kronk.Chat</a></li>
                <li><a href="#method-kronk-chatstreaming">Kronk.ChatStreaming</a></li>
                <li><a href="#method-kronk-chatstreaminghttp">Kronk.ChatStreamingHTTP</a></li>
                <li><a href="#method-kronk-embeddings">Kronk.Embeddings</a></li>
                <li><a href="#method-kronk-embeddingshttp">Kronk.EmbeddingsHTTP</a></li>
                <li><a href="#method-kronk-modelconfig">Kronk.ModelConfig</a></li>
                <li><a href="#method-kronk-modelinfo">Kronk.ModelInfo</a></li>
                <li><a href="#method-kronk-rerank">Kronk.Rerank</a></li>
                <li><a href="#method-kronk-rerankhttp">Kronk.RerankHTTP</a></li>
                <li><a href="#method-kronk-response">Kronk.Response</a></li>
                <li><a href="#method-kronk-responsestreaming">Kronk.ResponseStreaming</a></li>
                <li><a href="#method-kronk-responsestreaminghttp">Kronk.ResponseStreamingHTTP</a></li>
                <li><a href="#method-kronk-systeminfo">Kronk.SystemInfo</a></li>
                <li><a href="#method-kronk-tokenize">Kronk.Tokenize</a></li>
                <li><a href="#method-kronk-tokenizehttp">Kronk.TokenizeHTTP</a></li>
                <li><a href="#method-kronk-unload">Kronk.Unload</a></li>
                <li><a href="#method-loglevel-int">LogLevel.Int</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#constants" className="doc-index-header">Constants</a>
              <ul>
                <li><a href="#const-version">Version</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-discardlogger">DiscardLogger</a></li>
                <li><a href="#var-fmtlogger">FmtLogger</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
