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
                <code>func Init() error</code>
              </pre>
              <p className="doc-description">Init initializes the Kronk backend suport.</p>
            </div>

            <div className="doc-section" id="func-initwithsettings">
              <h4>InitWithSettings</h4>
              <pre className="code-block">
                <code>func InitWithSettings(libPath string, logLevel LogLevel) error</code>
              </pre>
              <p className="doc-description">InitWithSettings initializes the Kronk backend suport.</p>
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
                <code>func New(modelInstances int, cfg model.Config, opts ...Option) (*Kronk, error)</code>
              </pre>
              <p className="doc-description">New provides the ability to use models in a concurrently safe way. modelInstances represents the number of instances of the model to create. Unless you have more than 1 GPU, the recommended number of instances is 1.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

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
              <p className="doc-description">Option represents a functional option for configuring Kronk.</p>
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
              <p className="doc-description">Chat provides support to interact with an inference model.</p>
            </div>

            <div className="doc-section" id="method-kronk-chatstreaming">
              <h4>Kronk.ChatStreaming</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (&lt;-chan model.ChatResponse, error)</code>
              </pre>
              <p className="doc-description">ChatStreaming provides support to interact with an inference model.</p>
            </div>

            <div className="doc-section" id="method-kronk-chatstreaminghttp">
              <h4>Kronk.ChatStreamingHTTP</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error)</code>
              </pre>
              <p className="doc-description">ChatStreamingHTTP provides http handler support for a chat/completions call.</p>
            </div>

            <div className="doc-section" id="method-kronk-embeddings">
              <h4>Kronk.Embeddings</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Embeddings(ctx context.Context, input string) (model.EmbedReponse, error)</code>
              </pre>
              <p className="doc-description">Embeddings provides support to interact with an embedding model.</p>
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

            <div className="doc-section" id="method-kronk-systeminfo">
              <h4>Kronk.SystemInfo</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) SystemInfo() map[string]string</code>
              </pre>
              <p className="doc-description">SystemInfo returns system information.</p>
            </div>

            <div className="doc-section" id="method-kronk-unload">
              <h4>Kronk.Unload</h4>
              <pre className="code-block">
                <code>func (krn *Kronk) Unload(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Unload will close down all loaded models. You should call this only when you are completely done using the group.</p>
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
                <code>{`const Version = "1.9.9"`}</code>
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
	fmt.Println()
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
                <li><a href="#func-initwithsettings">InitWithSettings</a></li>
                <li><a href="#func-setfmtloggertraceid">SetFmtLoggerTraceID</a></li>
                <li><a href="#func-new">New</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-kronk">Kronk</a></li>
                <li><a href="#type-loglevel">LogLevel</a></li>
                <li><a href="#type-logger">Logger</a></li>
                <li><a href="#type-option">Option</a></li>
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
                <li><a href="#method-kronk-systeminfo">Kronk.SystemInfo</a></li>
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
