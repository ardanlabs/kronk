export default function DocsCLIServer() {
  return (
    <div>
      <div className="page-header">
        <h2>server</h2>
        <p>Manage the Kronk model server (start, stop, logs).</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk server &lt;command&gt; [flags]</code>
            </pre>
          </div>

          <div className="card" id="cmd-start">
            <h3>start</h3>
            <p className="doc-description">Start the Kronk model server.</p>
            <pre className="code-block">
              <code>kronk server start [flags]</code>
            </pre>

            <h4>General Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>-d, --detach</code></td>
                  <td>Run server in the background</td>
                </tr>
                <tr>
                  <td><code>--base-path &lt;string&gt;</code></td>
                  <td>Base path for kronk data (models, libraries, catalog, model_config)</td>
                </tr>
              </tbody>
            </table>

            <h4>Web Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--api-host &lt;string&gt;</code></td>
                  <td>API host address (e.g. <code>localhost:11435</code>)</td>
                </tr>
                <tr>
                  <td><code>--debug-host &lt;string&gt;</code></td>
                  <td>Debug host address (e.g. <code>localhost:8090</code>)</td>
                </tr>
                <tr>
                  <td><code>--read-timeout &lt;duration&gt;</code></td>
                  <td>HTTP read timeout (e.g. <code>30s</code>)</td>
                </tr>
                <tr>
                  <td><code>--write-timeout &lt;duration&gt;</code></td>
                  <td>HTTP write timeout (e.g. <code>15m</code>)</td>
                </tr>
                <tr>
                  <td><code>--idle-timeout &lt;duration&gt;</code></td>
                  <td>HTTP idle timeout (e.g. <code>1m</code>)</td>
                </tr>
                <tr>
                  <td><code>--shutdown-timeout &lt;duration&gt;</code></td>
                  <td>Server shutdown timeout (e.g. <code>1m</code>)</td>
                </tr>
                <tr>
                  <td><code>--cors-allowed-origins &lt;list&gt;</code></td>
                  <td>CORS allowed origins (comma-separated)</td>
                </tr>
              </tbody>
            </table>

            <h4>Auth Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--auth-enabled</code></td>
                  <td>Enable embedded local authentication</td>
                </tr>
                <tr>
                  <td><code>--auth-host &lt;string&gt;</code></td>
                  <td>External auth service host (when not using embedded)</td>
                </tr>
                <tr>
                  <td><code>--auth-issuer &lt;string&gt;</code></td>
                  <td>Local auth issuer name</td>
                </tr>
              </tbody>
            </table>

            <h4>Tracing Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--tempo-host &lt;string&gt;</code></td>
                  <td>Tempo host address (e.g. <code>localhost:4317</code>)</td>
                </tr>
                <tr>
                  <td><code>--tempo-service-name &lt;string&gt;</code></td>
                  <td>Tempo service name</td>
                </tr>
                <tr>
                  <td><code>--tempo-probability &lt;float&gt;</code></td>
                  <td>Tempo sampling probability (0.0-1.0)</td>
                </tr>
              </tbody>
            </table>

            <h4>Cache & Catalog Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--model-config-file &lt;string&gt;</code></td>
                  <td>Path to the model_config.yaml file</td>
                </tr>
                <tr>
                  <td><code>--model-instances &lt;int&gt;</code></td>
                  <td>Maximum number of concurrent model instances</td>
                </tr>
                <tr>
                  <td><code>--budget-percent &lt;int&gt;</code></td>
                  <td>Percentage (1..100) of detected GPU VRAM and system RAM that the resource manager may commit to loaded models (default 80)</td>
                </tr>
                <tr>
                  <td><code>--cache-ttl &lt;duration&gt;</code></td>
                  <td>Cache TTL duration (e.g. <code>5m</code>, <code>1h</code>)</td>
                </tr>
              </tbody>
            </table>

            <h4>Runtime Flags</h4>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--lib-path &lt;string&gt;</code></td>
                  <td>Path to the llama library</td>
                </tr>
                <tr>
                  <td><code>--lib-version &lt;string&gt;</code></td>
                  <td>Version of the llama library</td>
                </tr>
                <tr>
                  <td><code>--arch &lt;string&gt;</code></td>
                  <td>Architecture override (<code>amd64</code>, <code>arm64</code>)</td>
                </tr>
                <tr>
                  <td><code>--os &lt;string&gt;</code></td>
                  <td>OS override (<code>linux</code>, <code>darwin</code>, <code>windows</code>)</td>
                </tr>
                <tr>
                  <td><code>--processor &lt;string&gt;</code></td>
                  <td>Processor type (<code>cpu</code>, <code>cuda</code>, <code>metal</code>, <code>rocm</code>, <code>vulkan</code>)</td>
                </tr>
                <tr>
                  <td><code>--hf-token &lt;string&gt;</code></td>
                  <td>HuggingFace API token</td>
                </tr>
                <tr>
                  <td><code>--allow-upgrade</code></td>
                  <td>Allow automatic upgrades (default: <code>true</code>)</td>
                </tr>
                <tr>
                  <td><code>--llama-log &lt;int&gt;</code></td>
                  <td>Llama log level (<code>0</code>=off, <code>1</code>=on)</td>
                </tr>
                <tr>
                  <td><code>--insecure-logging</code></td>
                  <td>Enable logging of sensitive data (messages, model config)</td>
                </tr>
              </tbody>
            </table>

            <h4>Environment Variables</h4>
            <p>
              Each flag has a matching <code>KRONK_*</code> environment
              variable; pass <code>--help</code> to see the full live list.
              The most common ones are listed below.
            </p>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Variable</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>KRONK_BASE_PATH</code></td>
                  <td>Base path for kronk data (default: <code>$HOME/kronk</code>)</td>
                </tr>
                <tr>
                  <td><code>KRONK_WEB_API_HOST</code></td>
                  <td>API host address (default: <code>localhost:11435</code>)</td>
                </tr>
                <tr>
                  <td><code>KRONK_WEB_DEBUG_HOST</code></td>
                  <td>Debug host address</td>
                </tr>
                <tr>
                  <td><code>KRONK_WEB_READ_TIMEOUT</code> / <code>KRONK_WEB_WRITE_TIMEOUT</code> / <code>KRONK_WEB_IDLE_TIMEOUT</code> / <code>KRONK_WEB_SHUTDOWN_TIMEOUT</code></td>
                  <td>HTTP timeouts</td>
                </tr>
                <tr>
                  <td><code>KRONK_WEB_CORS_ALLOWED_ORIGINS</code></td>
                  <td>CORS allowed origins (comma-separated)</td>
                </tr>
                <tr>
                  <td><code>KRONK_AUTH_LOCAL_ENABLED</code></td>
                  <td>Enable embedded local authentication</td>
                </tr>
                <tr>
                  <td><code>KRONK_AUTH_LOCAL_ISSUER</code></td>
                  <td>Local auth issuer name</td>
                </tr>
                <tr>
                  <td><code>KRONK_AUTH_HOST</code></td>
                  <td>External auth service host (when not using embedded)</td>
                </tr>
                <tr>
                  <td><code>KRONK_TEMPO_HOST</code> / <code>KRONK_TEMPO_SERVICE_NAME</code> / <code>KRONK_TEMPO_PROBABILITY</code></td>
                  <td>Tracing configuration</td>
                </tr>
                <tr>
                  <td><code>KRONK_CACHE_MODEL_CONFIG_FILE</code></td>
                  <td>Path to model_config.yaml</td>
                </tr>
                <tr>
                  <td><code>KRONK_CACHE_MODEL_INSTANCES</code> / <code>KRONK_CACHE_BUDGET_PERCENT</code> / <code>KRONK_CACHE_TTL</code></td>
                  <td>Cache settings</td>
                </tr>
                <tr>
                  <td><code>KRONK_LIB_PATH</code> / <code>KRONK_LIB_VERSION</code></td>
                  <td>llama library path and version</td>
                </tr>
                <tr>
                  <td><code>KRONK_ARCH</code> / <code>KRONK_OS</code> / <code>KRONK_PROCESSOR</code></td>
                  <td>Platform overrides</td>
                </tr>
                <tr>
                  <td><code>KRONK_HF_TOKEN</code></td>
                  <td>HuggingFace API token</td>
                </tr>
                <tr>
                  <td><code>KRONK_ALLOW_UPGRADE</code></td>
                  <td>Allow automatic upgrades</td>
                </tr>
                <tr>
                  <td><code>KRONK_LLAMA_LOG</code></td>
                  <td>Llama log level (<code>0</code>=off, <code>1</code>=on)</td>
                </tr>
                <tr>
                  <td><code>KRONK_INSECURE_LOGGING</code></td>
                  <td>Enable logging of sensitive data</td>
                </tr>
              </tbody>
            </table>

            <h4>Example</h4>
            <pre className="code-block">
              <code>{`# Start the server in foreground
kronk server start

# Start the server in background
kronk server start -d

# Start with auth and a custom model_config.yaml
kronk server start --auth-enabled --model-config-file=/etc/kronk/model_config.yaml

# Start with tracing enabled
kronk server start --tempo-host=localhost:4317 --tempo-probability=1.0

# Show every flag and live env-var mapping
kronk server start --help`}</code>
            </pre>
          </div>

          <div className="card" id="cmd-stop">
            <h3>stop</h3>
            <p className="doc-description">Stop the Kronk model server by sending SIGTERM to the running process.</p>
            <pre className="code-block">
              <code>kronk server stop</code>
            </pre>
            <h5>Example</h5>
            <pre className="code-block">
              <code>{`# Stop the server
kronk server stop`}</code>
            </pre>
          </div>

          <div className="card" id="cmd-logs">
            <h3>logs</h3>
            <p className="doc-description">Stream the Kronk model server logs (<code>tail -f</code>).</p>
            <pre className="code-block">
              <code>kronk server logs</code>
            </pre>
            <h5>Example</h5>
            <pre className="code-block">
              <code>{`# Stream server logs
kronk server logs`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-start" className="doc-index-header">start</a>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-stop" className="doc-index-header">stop</a>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-logs" className="doc-index-header">logs</a>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
