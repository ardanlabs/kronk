export default function DocsCLIRun() {
  return (
    <div>
      <div className="page-header">
        <h2>run</h2>
        <p>Run an interactive chat session with a model.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk run &lt;MODEL_NAME&gt; [flags]</code>
            </pre>
          </div>

          <div className="card" id="subcommands">
            <h3>Subcommands</h3>

            <div className="doc-section" id="cmd-flags">
              <h4>flags</h4>
              <p className="doc-description">Available flags for the run command.</p>
              <pre className="code-block">
                <code>kronk run &lt;MODEL_NAME&gt; [flags]</code>
              </pre>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>--instances &lt;int&gt;</code></td>
                    <td>Number of model instances to load (default: 1)</td>
                  </tr>
                  <tr>
                    <td><code>--max-tokens &lt;int&gt;</code></td>
                    <td>Maximum tokens for response (default: 2048)</td>
                  </tr>
                  <tr>
                    <td><code>--temperature &lt;float&gt;</code></td>
                    <td>Temperature for sampling (default: 0.7)</td>
                  </tr>
                  <tr>
                    <td><code>--top-p &lt;float&gt;</code></td>
                    <td>Top-p for sampling (default: 0.9)</td>
                  </tr>
                  <tr>
                    <td><code>--top-k &lt;int&gt;</code></td>
                    <td>Top-k for sampling (default: 40)</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, catalogs, templates)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Environment Variables</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Variable</th>
                    <th>Default</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Start an interactive chat with a model
kronk run Qwen3-8B-Q8_0

# Run with custom sampling parameters
kronk run Qwen3-8B-Q8_0 --temperature 0.5 --top-p 0.95

# Run with higher token limit
kronk run Qwen3-8B-Q8_0 --max-tokens 4096`}</code>
              </pre>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#subcommands" className="doc-index-header">Subcommands</a>
              <ul>
                <li><a href="#cmd-flags">flags</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
