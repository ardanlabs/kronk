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
              <h5>Model Configuration</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>--jinja-file &lt;string&gt;</code></td>
                    <td>Path to a custom Jinja template file</td>
                  </tr>
                  <tr>
                    <td><code>--context-window &lt;int&gt;</code></td>
                    <td>Context window size in tokens</td>
                  </tr>
                  <tr>
                    <td><code>--flash-attention &lt;string&gt;</code></td>
                    <td>Flash attention mode (on, off, auto)</td>
                  </tr>
                  <tr>
                    <td><code>--ngpu-layers &lt;int&gt;</code></td>
                    <td>Number of layers to offload to GPU (-1 = CPU only)</td>
                  </tr>
                  <tr>
                    <td><code>--cache-type-k &lt;string&gt;</code></td>
                    <td>KV cache type for keys (f16, q8_0, q4_0, etc.)</td>
                  </tr>
                  <tr>
                    <td><code>--cache-type-v &lt;string&gt;</code></td>
                    <td>KV cache type for values (f16, q8_0, q4_0, etc.)</td>
                  </tr>
                  <tr>
                    <td><code>--nbatch &lt;int&gt;</code></td>
                    <td>Logical batch size for processing</td>
                  </tr>
                  <tr>
                    <td><code>--nubatch &lt;int&gt;</code></td>
                    <td>Physical micro-batch size for prompt ingestion</td>
                  </tr>
                </tbody>
              </table>

              <h5>Sampling Parameters</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>--max-tokens &lt;int&gt;</code></td>
                    <td>Maximum tokens for response</td>
                  </tr>
                  <tr>
                    <td><code>--temperature &lt;float&gt;</code></td>
                    <td>Temperature for sampling</td>
                  </tr>
                  <tr>
                    <td><code>--top-p &lt;float&gt;</code></td>
                    <td>Top-p for sampling</td>
                  </tr>
                  <tr>
                    <td><code>--top-k &lt;int&gt;</code></td>
                    <td>Top-k for sampling</td>
                  </tr>
                  <tr>
                    <td><code>--min-p &lt;float&gt;</code></td>
                    <td>Min-p for sampling</td>
                  </tr>
                  <tr>
                    <td><code>--repeat-penalty &lt;float&gt;</code></td>
                    <td>Repetition penalty</td>
                  </tr>
                  <tr>
                    <td><code>--frequency-penalty &lt;float&gt;</code></td>
                    <td>Frequency penalty</td>
                  </tr>
                  <tr>
                    <td><code>--presence-penalty &lt;float&gt;</code></td>
                    <td>Presence penalty</td>
                  </tr>
                  <tr>
                    <td><code>--enable-thinking &lt;string&gt;</code></td>
                    <td>Enable thinking/reasoning (true, false)</td>
                  </tr>
                  <tr>
                    <td><code>--reasoning-effort &lt;string&gt;</code></td>
                    <td>Reasoning effort level (low, medium, high)</td>
                  </tr>
                </tbody>
              </table>

              <h5>General</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
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

# Run with a custom Jinja template
kronk run Qwen3-8B-Q8_0 --jinja-file /path/to/template.jinja

# Run with custom context window and flash attention
kronk run Qwen3-8B-Q8_0 --context-window 16384 --flash-attention auto

# Run with custom sampling parameters
kronk run Qwen3-8B-Q8_0 --temperature 0.5 --top-p 0.95

# Run with thinking enabled
kronk run Qwen3-8B-Q8_0 --enable-thinking true --reasoning-effort high

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
