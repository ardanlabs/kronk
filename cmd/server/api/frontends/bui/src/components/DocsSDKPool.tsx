import { useEffect } from 'react';
import { useLocation } from 'react-router-dom';

export default function DocsSDKPool() {
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
        <h2>Pool Package</h2>
        <p>Package pool manages a pool of kronk APIs for specific models. Used by the model server to manage the number of models that are maintained in memory at any given time.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card">
            <h3>Import</h3>
            <pre className="code-block">
              <code>import "github.com/ardanlabs/kronk/sdk/pool"</code>
            </pre>
          </div>

          <div className="card" id="functions">
            <h3>Functions</h3>

            <div className="doc-section" id="func-new">
              <h4>New</h4>
              <pre className="code-block">
                <code>func New(cfg Config) (*Pool, error)</code>
              </pre>
              <p className="doc-description">New constructs the manager for use.</p>
            </div>
          </div>

          <div className="card" id="types">
            <h3>Types</h3>

            <div className="doc-section" id="type-config">
              <h4>Config</h4>
              <pre className="code-block">
                <code>{`type Config struct {
	Log             kronk.Logger
	BasePath        string
	ModelConfigFile string
	ModelsInCache   int
	CacheTTL        time.Duration
	InsecureLogging bool
}`}</code>
              </pre>
              <p className="doc-description">Config represents setting for the kronk manager. ModelsInCache: Defines the maximum number of unique models that will be available at a time. Defaults to 2 if the value is 0. CacheTTL: Defines the time an existing model can live in the pool without being used. Defaults to 5 minutes if the value is 0. InsecureLogging: When true, logs potentially sensitive data such as message content and detailed model configuration.</p>
            </div>

            <div className="doc-section" id="type-modeldetail">
              <h4>ModelDetail</h4>
              <pre className="code-block">
                <code>{`type ModelDetail struct {
	ID            string
	OwnedBy       string
	ModelFamily   string
	Size          int64
	VRAMTotal     int64
	KVCache       int64
	Slots         int
	ExpiresAt     time.Time
	ActiveStreams int
}`}</code>
              </pre>
              <p className="doc-description">ModelDetail provides details for the models in the pool.</p>
            </div>

            <div className="doc-section" id="type-pool">
              <h4>Pool</h4>
              <pre className="code-block">
                <code>{`type Pool struct {
	// Has unexported fields.
}`}</code>
              </pre>
              <p className="doc-description">Pool manages a set of Kronk APIs for use. It maintains a pool of these APIs and will unload over time if not in use.</p>
            </div>
          </div>

          <div className="card" id="methods">
            <h3>Methods</h3>

            <div className="doc-section" id="method-pool-aquirecustom">
              <h4>Pool.AquireCustom</h4>
              <pre className="code-block">
                <code>func (p *Pool) AquireCustom(ctx context.Context, key string, cfg model.Config) (*kronk.Kronk, error)</code>
              </pre>
              <p className="doc-description">AquireCustom will provide a kronk API for a model using a pre-built config. This bypasses the normal catalog resolution path. The key should use format &lt;modelID&gt;/playground/&lt;session_id&gt; so that ModelStatus() can still match playground sessions to locally installed models.</p>
            </div>

            <div className="doc-section" id="method-pool-aquiremodel">
              <h4>Pool.AquireModel</h4>
              <pre className="code-block">
                <code>func (p *Pool) AquireModel(ctx context.Context, modelID string) (*kronk.Kronk, error)</code>
              </pre>
              <p className="doc-description">AquireModel will provide a kronk API for the specified model. If the model is not in the pool, an API for the model will be created.</p>
            </div>

            <div className="doc-section" id="method-pool-getexisting">
              <h4>Pool.GetExisting</h4>
              <pre className="code-block">
                <code>func (p *Pool) GetExisting(key string) (*kronk.Kronk, bool)</code>
              </pre>
              <p className="doc-description">GetExisting returns a pooled model if it exists, without creating one.</p>
            </div>

            <div className="doc-section" id="method-pool-invalidate">
              <h4>Pool.Invalidate</h4>
              <pre className="code-block">
                <code>func (p *Pool) Invalidate(key string)</code>
              </pre>
              <p className="doc-description">Invalidate removes a single entry from the pool, triggering unload.</p>
            </div>

            <div className="doc-section" id="method-pool-modelconfig">
              <h4>Pool.ModelConfig</h4>
              <pre className="code-block">
                <code>func (p *Pool) ModelConfig() map[string]models.ModelConfig</code>
              </pre>
              <p className="doc-description">ModelConfig returns the loaded per-model configuration overrides.</p>
            </div>

            <div className="doc-section" id="method-pool-modelstatus">
              <h4>Pool.ModelStatus</h4>
              <pre className="code-block">
                <code>func (p *Pool) ModelStatus() ([]ModelDetail, error)</code>
              </pre>
              <p className="doc-description">ModelStatus returns information about the current models in the pool.</p>
            </div>

            <div className="doc-section" id="method-pool-shutdown">
              <h4>Pool.Shutdown</h4>
              <pre className="code-block">
                <code>func (p *Pool) Shutdown(ctx context.Context) error</code>
              </pre>
              <p className="doc-description">Shutdown releases all apis from the pool and performs a proper unloading.</p>
            </div>
          </div>

          <div className="card" id="variables">
            <h3>Variables</h3>

            <div className="doc-section" id="var-errserverbusy">
              <h4>ErrServerBusy</h4>
              <pre className="code-block">
                <code>{`var ErrServerBusy = errors.New("server busy: all model slots have active requests")`}</code>
              </pre>
              <p className="doc-description">ErrServerBusy is returned when all model slots are occupied with active streams.</p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#functions" className="doc-index-header">Functions</a>
              <ul>
                <li><a href="#func-new">New</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#types" className="doc-index-header">Types</a>
              <ul>
                <li><a href="#type-config">Config</a></li>
                <li><a href="#type-modeldetail">ModelDetail</a></li>
                <li><a href="#type-pool">Pool</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#methods" className="doc-index-header">Methods</a>
              <ul>
                <li><a href="#method-pool-aquirecustom">Pool.AquireCustom</a></li>
                <li><a href="#method-pool-aquiremodel">Pool.AquireModel</a></li>
                <li><a href="#method-pool-getexisting">Pool.GetExisting</a></li>
                <li><a href="#method-pool-invalidate">Pool.Invalidate</a></li>
                <li><a href="#method-pool-modelconfig">Pool.ModelConfig</a></li>
                <li><a href="#method-pool-modelstatus">Pool.ModelStatus</a></li>
                <li><a href="#method-pool-shutdown">Pool.Shutdown</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#variables" className="doc-index-header">Variables</a>
              <ul>
                <li><a href="#var-errserverbusy">ErrServerBusy</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
