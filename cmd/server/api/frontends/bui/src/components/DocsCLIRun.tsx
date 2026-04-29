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
            <p>
              Provides a simple interactive REPL for chatting directly with a
              GGUF model without starting the full Model Server. It loads the
              model, applies the chat template, and enters a REPL loop for
              conversation.
            </p>
            <h5>Features</h5>
            <ul>
              <li>Interactive chat with streaming responses</li>
              <li>Customizable Jinja chat templates</li>
              <li>Fine-grained control over inference parameters</li>
              <li>No server required — runs directly on your machine</li>
            </ul>
          </div>

          <div className="card" id="model-config">
            <h3>Model Configuration Flags</h3>
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
                  <td>Flash attention mode (<code>on</code>, <code>off</code>, <code>auto</code>)</td>
                </tr>
                <tr>
                  <td><code>--ngpu-layers &lt;int&gt;</code></td>
                  <td>Number of layers to offload to GPU (<code>-1</code> = CPU only)</td>
                </tr>
                <tr>
                  <td><code>--devices &lt;string&gt;</code></td>
                  <td>Comma-separated list of devices (e.g. <code>CUDA0,CUDA1</code>)</td>
                </tr>
                <tr>
                  <td><code>--main-gpu &lt;int&gt;</code></td>
                  <td>Main GPU index for single-GPU mode</td>
                </tr>
                <tr>
                  <td><code>--tensor-split &lt;string&gt;</code></td>
                  <td>Comma-separated tensor split ratios (e.g. <code>0.6,0.4</code>)</td>
                </tr>
                <tr>
                  <td><code>--split-mode &lt;string&gt;</code></td>
                  <td>GPU split mode (<code>none</code>, <code>layer</code>, <code>row</code>)</td>
                </tr>
                <tr>
                  <td><code>--cache-type-k &lt;string&gt;</code></td>
                  <td>KV cache type for keys (<code>f16</code>, <code>q8_0</code>, <code>q4_0</code>, etc.)</td>
                </tr>
                <tr>
                  <td><code>--cache-type-v &lt;string&gt;</code></td>
                  <td>KV cache type for values (<code>f16</code>, <code>q8_0</code>, <code>q4_0</code>, etc.)</td>
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
          </div>

          <div className="card" id="sampling">
            <h3>Sampling Parameter Flags</h3>
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
                  <td>Temperature for sampling (0.0-2.0)</td>
                </tr>
                <tr>
                  <td><code>--top-p &lt;float&gt;</code></td>
                  <td>Top-p (nucleus) for sampling</td>
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
                  <td>Enable thinking/reasoning mode (<code>true</code>, <code>false</code>)</td>
                </tr>
                <tr>
                  <td><code>--reasoning-effort &lt;string&gt;</code></td>
                  <td>Reasoning effort level (<code>low</code>, <code>medium</code>, <code>high</code>)</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="general">
            <h3>General Flags</h3>
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
                  <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="env">
            <h3>Environment Variables</h3>
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
                  <td>Base directory for kronk data (models, libraries, catalog, model_config)</td>
                </tr>
                <tr>
                  <td><code>KRONK_MODELS</code></td>
                  <td>$HOME/.kronk/models</td>
                  <td>Path to the models directory</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# Start an interactive chat with a model
kronk run Qwen3-8B-Q8_0

# Use a custom Jinja template and a larger context window
kronk run Llama-3.3-70B-Instruct-Q8_0 --jinja-file=/tmp/template.j2 --context-window=32764

# Run with GPU offloading and adjusted sampling
kronk run Qwen3-8B-Q8_0 --ngpu-layers=35 --temperature=0.7

# Run with custom sampling parameters
kronk run Qwen3-8B-Q8_0 --temperature=0.5 --top-p=0.95

# Run with thinking enabled
kronk run Qwen3-8B-Q8_0 --enable-thinking=true --reasoning-effort=high

# Run with higher token limit
kronk run Qwen3-8B-Q8_0 --max-tokens=4096`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#model-config" className="doc-index-header">Model Configuration Flags</a>
            </div>
            <div className="doc-index-section">
              <a href="#sampling" className="doc-index-header">Sampling Parameter Flags</a>
            </div>
            <div className="doc-index-section">
              <a href="#general" className="doc-index-header">General Flags</a>
            </div>
            <div className="doc-index-section">
              <a href="#env" className="doc-index-header">Environment Variables</a>
            </div>
            <div className="doc-index-section">
              <a href="#examples" className="doc-index-header">Examples</a>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
