import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKMalina() {
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
        <h2>Malina Package</h2>
        <p>Package malina provides a concurrency-safe API for generating images with stable-diffusion.cpp through the Malina raw bindings.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/malina"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-init">
              <h4>Init</h4>
              <pre className="code-block">
                <code>func Init(opts ...InitOption) error</code>
              </pre>
              <p className="doc-description">Init registers Malina tooling, then loads stable-diffusion.cpp and its dynamic backends. KRONK_MALINA_LIB_PATH (or legacy MALINA_LIB) is used when no library path is supplied.</p>
            </div>

            <div className="doc-section" id="func-initialized">
              <h4>Initialized</h4>
              <pre className="code-block">
                <code>func Initialized() bool</code>
              </pre>
              <p className="doc-description">Initialized reports whether the Malina backend has been successfully initialized.</p>
            </div>

            <div className="doc-section" id="func-setfmtloggertraceid">
              <h4>SetFmtLoggerTraceID</h4>
              <pre className="code-block">
                <code>func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context</code>
              </pre>
              <p className="doc-description">SetFmtLoggerTraceID allows you to set a trace id on the context that can be included in FmtLogger output.</p>
            </div>

            <div className="doc-section" id="func-new">
              <h4>New</h4>
              <pre className="code-block">
                <code>func New(opts ...model.Option) (*Malina, error)</code>
              </pre>
              <p className="doc-description">New provides pooled image generation using a background model-loading context.</p>
            </div>

            <div className="doc-section" id="func-newwithcontext">
              <h4>NewWithContext</h4>
              <pre className="code-block">
                <code>func NewWithContext(ctx context.Context, opts ...model.Option) (*Malina, error)</code>
              </pre>
              <p className="doc-description">NewWithContext provides pooled image generation and loads every configured model context using ctx.</p>
            </div>

            <div className="doc-section" id="func-systeminfo">
              <h4>SystemInfo</h4>
              <pre className="code-block">
                <code>func SystemInfo() (SystemDiagnostics, error)</code>
              </pre>
              <p className="doc-description">SystemInfo returns native library and host diagnostics after initialization.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-initoption">
              <h4>InitOption</h4>
              <pre className="code-block">
                <code>{`type InitOption func(*initOptions)`}</code>
              </pre>
              <p className="doc-description">InitOption represents options for configuring Init.</p>
            </div>

            <div className="doc-section" id="type-loglevel">
              <h4>LogLevel</h4>
              <pre className="code-block">
                <code>{`type LogLevel = applog.LogLevel`}</code>
              </pre>
              <p className="doc-description">LogLevel represents the native logging level.</p>
            </div>

            <div className="doc-section" id="type-logger">
              <h4>Logger</h4>
              <pre className="code-block">
                <code>{`type Logger = applog.Logger`}</code>
              </pre>
              <p className="doc-description">Logger provides a function for logging messages from different APIs.</p>
            </div>

            <div className="doc-section" id="type-malina">
              <h4>Malina</h4>
              <pre className="code-block">
                <code>{`type Malina struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Malina provides a concurrency-safe API around a pool of reusable native model contexts. Each context performs one generation at a time.</p>
            </div>

            <div className="doc-section" id="type-progressfunc">
              <h4>ProgressFunc</h4>
              <pre className="code-block">
                <code>{`type ProgressFunc func(step int, steps int, secondsPerStep float32)`}</code>
              </pre>
              <p className="doc-description">ProgressFunc receives process-global model loading and image generation progress. SecondsPerStep is the average elapsed time per completed step. The function can be called concurrently and must be concurrency-safe.</p>
            </div>

            <div className="doc-section" id="type-systemdiagnostics">
              <h4>SystemDiagnostics</h4>
              <pre className="code-block">
                <code>{`type SystemDiagnostics struct {
	NativeVersion      string
	PhysicalCores      int32
	BackendDeviceCount int
	Description        string
}`}</code>
              </pre>
              <p className="doc-description">SystemDiagnostics contains native library and host diagnostics.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-malina-activegenerations">
              <h4>Malina.ActiveGenerations</h4>
              <pre className="code-block">
                <code>func (m *Malina) ActiveGenerations() int</code>
              </pre>
              <p className="doc-description">ActiveGenerations returns the number of running and queued generation calls.</p>
            </div>

            <div className="doc-section" id="method-malina-generate">
              <h4>Malina.Generate</h4>
              <pre className="code-block">
                <code>func (m *Malina) Generate(ctx context.Context, params model.GenerateParams) (model.GeneratedImage, error)</code>
              </pre>
              <p className="doc-description">Generate admits and synchronously executes one image generation. Waiting for admission is cancellable. Once native generation starts, the call waits for native completion before returning a cancellation error so the context cannot be reused or freed while native code is active.</p>
            </div>

            <div className="doc-section" id="method-malina-modelconfig">
              <h4>Malina.ModelConfig</h4>
              <pre className="code-block">
                <code>func (m *Malina) ModelConfig() model.Config</code>
              </pre>
              <p className="doc-description">ModelConfig returns a copy of the resolved model configuration.</p>
            </div>

            <div className="doc-section" id="method-malina-modelinfo">
              <h4>Malina.ModelInfo</h4>
              <pre className="code-block">
                <code>func (m *Malina) ModelInfo() model.ModelInfo</code>
              </pre>
              <p className="doc-description">ModelInfo returns descriptive information for the loaded model.</p>
            </div>

            <div className="doc-section" id="method-malina-ready">
              <h4>Malina.Ready</h4>
              <pre className="code-block">
                <code>func (m *Malina) Ready() bool</code>
              </pre>
              <p className="doc-description">Ready reports whether the model can accept generation requests.</p>
            </div>

            <div className="doc-section" id="method-malina-systeminfo">
              <h4>Malina.SystemInfo</h4>
              <pre className="code-block">
                <code>func (m *Malina) SystemInfo() SystemDiagnostics</code>
              </pre>
              <p className="doc-description">SystemInfo returns native library and host diagnostics.</p>
            </div>

            <div className="doc-section" id="method-malina-unload">
              <h4>Malina.Unload</h4>
              <pre className="code-block">
                <code>func (m *Malina) Unload(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Unload stops admission and waits for safe native context release. If ctx expires, cleanup continues and a later call can wait for its completion.</p>
            </div>
          </div>

          <div className="card" id="constants">
            <h3>Constants</h3>

            <div className="doc-section" id="const-logsilent">
              <h4>LogSilent</h4>
              <pre className="code-block">
                <code>{`const (
	LogSilent = applog.LogSilent
	LogNormal = applog.LogNormal
)`}</code>
              </pre>
              <p className="doc-description">Set of logging levels supported by stable-diffusion.cpp and GGML.</p>
            </div>

            <div className="doc-section" id="const-version">
              <h4>Version</h4>
              <pre className="code-block">
                <code>{`const Version = kronk.Version`}</code>
              </pre>
              <p className="doc-description">Version contains the current version of the Malina SDK package.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-variable">
              <h4>Variable</h4>
              <pre className="code-block">
                <code>{`var (`}</code>
              </pre>
              <p className="doc-description">// ErrInvalidRequest identifies invalid generation parameters. ErrInvalidRequest = model.ErrInvalidRequest // ErrAdmissionTimeout identifies expiration while waiting for admission. ErrAdmissionTimeout = errors.New("generation admission timed out") // ErrClosed identifies use after unloading has begun. ErrClosed = errors.New("malina is closed") // ErrPoisoned identifies a terminal native generation failure. ErrPoisoned = errors.New("malina is poisoned")</p>
            </div>

            <div className="doc-section" id="var-discardlogger">
              <h4>DiscardLogger</h4>
              <pre className="code-block">
                <code>{`var DiscardLogger = applog.DiscardLogger`}</code>
              </pre>
              <p className="doc-description">DiscardLogger discards logging.</p>
            </div>

            <div className="doc-section" id="var-fmtlogger">
              <h4>FmtLogger</h4>
              <pre className="code-block">
                <code>{`var FmtLogger = applog.FmtLogger`}</code>
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
                <li><a href="#func-initialized">Initialized</a></li>
                <li><a href="#func-setfmtloggertraceid">SetFmtLoggerTraceID</a></li>
                <li><a href="#func-new">New</a></li>
                <li><a href="#func-newwithcontext">NewWithContext</a></li>
                <li><a href="#func-systeminfo">SystemInfo</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-initoption">InitOption</a></li>
                <li><a href="#type-loglevel">LogLevel</a></li>
                <li><a href="#type-logger">Logger</a></li>
                <li><a href="#type-malina">Malina</a></li>
                <li><a href="#type-progressfunc">ProgressFunc</a></li>
                <li><a href="#type-systemdiagnostics">SystemDiagnostics</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-malina-activegenerations">Malina.ActiveGenerations</a></li>
                <li><a href="#method-malina-generate">Malina.Generate</a></li>
                <li><a href="#method-malina-modelconfig">Malina.ModelConfig</a></li>
                <li><a href="#method-malina-modelinfo">Malina.ModelInfo</a></li>
                <li><a href="#method-malina-ready">Malina.Ready</a></li>
                <li><a href="#method-malina-systeminfo">Malina.SystemInfo</a></li>
                <li><a href="#method-malina-unload">Malina.Unload</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#constants" className="doc-index-header">Constants</a>
              <ul>
                <li><a href="#const-logsilent">LogSilent</a></li>
                <li><a href="#const-version">Version</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-variable">Variable</a></li>
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
