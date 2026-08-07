export default function DocsCLILaunch() {
  return (
    <div>
      <div className="page-header">
        <h2>launch</h2>
        <p>Launch a coding agent configured to use the running Kronk server and its installed chat models.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk launch &lt;agent&gt; [--model &lt;model&gt;] [-- &lt;agent arguments&gt;]</code>
            </pre>
            <p>The Kronk server must already be running. The launcher discovers installed chat-capable models, configures the selected agent to use Kronk, and starts the agent. If the agent is not installed, an interactive launch offers to install it using the platform recipe bundled with Kronk.</p>
            <p>When no curated launch model is installed and no explicit model is selected, an interactive launch offers to download one. A non-interactive launch never installs an agent or downloads a model without confirmation.</p>
          </div>

          <div className="card" id="agents">
            <h3>Supported Agents</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Agent</th>
                  <th>Integration</th>
                </tr>
              </thead>
              <tbody>
                <tr><td><code>opencode</code></td><td>OpenCode</td></tr>
                <tr><td><code>claude</code></td><td>Claude Code</td></tr>
                <tr><td><code>codex</code></td><td>Codex CLI</td></tr>
                <tr><td><code>copilot</code></td><td>GitHub Copilot CLI</td></tr>
                <tr><td><code>pi</code></td><td>Pi</td></tr>
                <tr><td><code>openclaw</code></td><td>OpenClaw</td></tr>
                <tr><td><code>hermes</code></td><td>Hermes Agent</td></tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="models">
            <h3>Model Selection</h3>
            <p>Without <code>--model</code>, launch prefers the first installed curated coding model, then an installed profile variant, and finally the first installed chat model. An explicit value must resolve to an installed chat model.</p>
            <p><code>--model</code> accepts a complete installed model ID or one of these curated aliases:</p>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Alias</th>
                  <th>Model</th>
                  <th>Quantization</th>
                </tr>
              </thead>
              <tbody>
                <tr><td><code>qwen</code></td><td>Qwen3.6-35B-A3B</td><td><code>Q8_K_XL</code></td></tr>
                <tr><td><code>qwen-mtp</code></td><td>Qwen3.6-35B-A3B (MTP)</td><td><code>Q8_K_XL</code></td></tr>
                <tr><td><code>gemma</code></td><td>Gemma-4-26B-A4B-it</td><td><code>Q8_K_XL</code></td></tr>
                <tr><td><code>qwen-8b</code></td><td>Qwen3-8B</td><td><code>Q8_0</code></td></tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="flags">
            <h3>Flags</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--model &lt;string&gt;</code></td>
                  <td>Curated alias or complete ID of an installed chat model</td>
                </tr>
                <tr>
                  <td><code>--base-path &lt;string&gt;</code></td>
                  <td>Base path for Kronk data — persistent global flag</td>
                </tr>
                <tr>
                  <td><code>-- &lt;arguments&gt;</code></td>
                  <td>Pass every argument after <code>--</code> directly to the launched agent</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="environment">
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
                  <td><code>KRONK_WEB_API_HOST</code></td>
                  <td>localhost:11435</td>
                  <td>Address of the running Kronk server</td>
                </tr>
                <tr>
                  <td><code>KRONK_TOKEN</code></td>
                  <td></td>
                  <td>Bearer token used for model discovery and passed to supported agent configurations when authentication is enabled</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# Launch OpenCode with the preferred installed model
kronk launch opencode

# Launch Claude Code with a curated model alias
kronk launch claude --model qwen

# Launch Codex with a complete installed model ID
kronk launch codex --model Qwen3-8B-Q8_0

# Pass arguments directly to the agent
kronk launch opencode -- --help`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section"><a href="#usage" className="doc-index-header">Usage</a></div>
            <div className="doc-index-section"><a href="#agents" className="doc-index-header">Supported Agents</a></div>
            <div className="doc-index-section"><a href="#models" className="doc-index-header">Model Selection</a></div>
            <div className="doc-index-section"><a href="#flags" className="doc-index-header">Flags</a></div>
            <div className="doc-index-section"><a href="#environment" className="doc-index-header">Environment</a></div>
            <div className="doc-index-section"><a href="#examples" className="doc-index-header">Examples</a></div>
          </div>
        </nav>
      </div>
    </div>
  );
}
