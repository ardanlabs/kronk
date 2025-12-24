export default function DocsSDKKronk() {
  return (
    <div>
      <div className="page-header">
        <h2>Kronk Package</h2>
        <p>Core SDK for interacting with models using llama.cpp via yzma</p>
      </div>

      <div className="card">
        <h3>Import</h3>
        <pre className="code-block">
          <code>import "github.com/ardanlabs/kronk/sdk/kronk"</code>
        </pre>
      </div>

      <div className="card">
        <h3>Constants</h3>
        <div className="doc-section">
          <h4>Version</h4>
          <pre className="code-block">
            <code>const Version = "1.9.1"</code>
          </pre>
          <p className="doc-description">Contains the current version of the kronk package.</p>
        </div>
      </div>

      <div className="card">
        <h3>Types</h3>

        <div className="doc-section">
          <h4>Kronk</h4>
          <pre className="code-block">
            <code>type Kronk struct &#123;
    // contains filtered or unexported fields
&#125;</code>
          </pre>
          <p className="doc-description">
            Kronk provides a concurrently safe API for using llama.cpp to access models.
          </p>
        </div>

        <div className="doc-section">
          <h4>Option</h4>
          <pre className="code-block">
            <code>type Option func(*options)</code>
          </pre>
          <p className="doc-description">
            Option represents a functional option for configuring Kronk.
          </p>
        </div>

        <div className="doc-section">
          <h4>Logger</h4>
          <pre className="code-block">
            <code>type Logger interface &#123;
    Info(ctx context.Context, msg string, args ...any)
    Error(ctx context.Context, msg string, args ...any)
&#125;</code>
          </pre>
          <p className="doc-description">
            Logger interface used for logging in HTTP handlers.
          </p>
        </div>
      </div>

      <div className="card">
        <h3>Functions</h3>

        <div className="doc-section">
          <h4>New</h4>
          <pre className="code-block">
            <code>func New(modelInstances int, cfg model.Config, opts ...Option) (*Kronk, error)</code>
          </pre>
          <p className="doc-description">
            New provides the ability to use models in a concurrently safe way.
          </p>
          <p className="doc-description">
            <strong>modelInstances</strong> represents the number of instances of the model to create.
            Unless you have more than 1 GPU, the recommended number of instances is 1.
          </p>
        </div>

        <div className="doc-section">
          <h4>WithTemplateRetriever</h4>
          <pre className="code-block">
            <code>func WithTemplateRetriever(templates model.TemplateRetriever) Option</code>
          </pre>
          <p className="doc-description">
            WithTemplateRetriever sets a custom Github repo for templates.
            If not set, the default repo will be used.
          </p>
        </div>
      </div>

      <div className="card">
        <h3>Methods</h3>

        <div className="doc-section">
          <h4>ModelConfig</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) ModelConfig() model.Config</code>
          </pre>
          <p className="doc-description">
            ModelConfig returns a copy of the configuration being used. This may be different from the
            configuration passed to New() if the model has overridden any of the settings.
          </p>
        </div>

        <div className="doc-section">
          <h4>ModelInfo</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) ModelInfo() model.ModelInfo</code>
          </pre>
          <p className="doc-description">ModelInfo returns the model information.</p>
        </div>

        <div className="doc-section">
          <h4>SystemInfo</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) SystemInfo() map[string]string</code>
          </pre>
          <p className="doc-description">SystemInfo returns system information.</p>
        </div>

        <div className="doc-section">
          <h4>ActiveStreams</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) ActiveStreams() int</code>
          </pre>
          <p className="doc-description">ActiveStreams returns the number of active streams.</p>
        </div>

        <div className="doc-section">
          <h4>Unload</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) Unload(ctx context.Context) error</code>
          </pre>
          <p className="doc-description">
            Unload will close down all loaded models. You should call this only when you are completely
            done using the group.
          </p>
        </div>

        <div className="doc-section">
          <h4>Chat</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) Chat(ctx context.Context, d model.D) (model.ChatResponse, error)</code>
          </pre>
          <p className="doc-description">
            Chat provides support to interact with an inference model. The context must have a deadline
            set with a reasonable timeout.
          </p>
        </div>

        <div className="doc-section">
          <h4>ChatStreaming</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) ChatStreaming(ctx context.Context, d model.D) (&lt;-chan model.ChatResponse, error)</code>
          </pre>
          <p className="doc-description">
            ChatStreaming provides support to interact with an inference model using streaming responses.
            The context must have a deadline set with a reasonable timeout.
          </p>
        </div>

        <div className="doc-section">
          <h4>ChatStreamingHTTP</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) ChatStreamingHTTP(ctx context.Context, w http.ResponseWriter, d model.D) (model.ChatResponse, error)</code>
          </pre>
          <p className="doc-description">
            ChatStreamingHTTP provides HTTP handler support for a chat/completions call. Supports both
            streaming and non-streaming responses based on the "stream" field in the request.
          </p>
        </div>

        <div className="doc-section">
          <h4>Embeddings</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) Embeddings(ctx context.Context, input string) (model.EmbedReponse, error)</code>
          </pre>
          <p className="doc-description">
            Embeddings provides support to interact with an embedding model. Returns an error if the
            model doesn't support embedding.
          </p>
        </div>

        <div className="doc-section">
          <h4>EmbeddingsHTTP</h4>
          <pre className="code-block">
            <code>func (krn *Kronk) EmbeddingsHTTP(ctx context.Context, log Logger, w http.ResponseWriter, d model.D) (model.EmbedReponse, error)</code>
          </pre>
          <p className="doc-description">
            EmbeddingsHTTP provides HTTP handler support for an embeddings call.
          </p>
        </div>
      </div>
    </div>
  );
}
