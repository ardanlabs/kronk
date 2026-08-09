export default function DocsCLILaunch() {
  return (
    <div>
      <div className="page-header">
        <h2>launch</h2>
        <p>Launch an isolated OpenCode session connected to a running Kronk server.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk launch opencode &lt;model&gt; [--host &lt;IP:PORT&gt;] [-- &lt;OpenCode arguments&gt;]</code>
            </pre>
            <p>OpenCode is the only supported client. It uses the model ID exactly as provided, with Kronk responsible for resolving it to a downloaded model and optional configuration profile. Launch never starts a server or downloads models.</p>
            <p>Accepted model ID forms are <code>modelName</code>, <code>provider/modelName</code>, and <code>provider/modelName/userName</code>.</p>
          </div>

          <div className="card" id="flags">
            <h3>Flags</h3>
            <table className="flags-table">
              <thead>
                <tr>
                  <th>Flag</th>
                  <th>Default</th>
                  <th>Description</th>
                </tr>
              </thead>
              <tbody>
                <tr>
                  <td><code>--host &lt;IP:PORT&gt;</code></td>
                  <td><code>localhost:11435</code></td>
                  <td>Address of the running Kronk server.</td>
                </tr>
                <tr>
                  <td><code>-- &lt;arguments&gt;</code></td>
                  <td>None</td>
                  <td>Pass all arguments after <code>--</code> directly to OpenCode.</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="prerequisites">
            <h3>Server and Model Prerequisites</h3>
            <p>Start Kronk before launching OpenCode. Launch connects to <code>localhost:11435</code> by default; use <code>--host</code> when the server runs elsewhere. It verifies the server and selected model through <code>GET /v1/models</code> before starting OpenCode.</p>
            <pre className="code-block"><code>{`kronk server start
kronk launch opencode unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT

# Connect to a server at another address
kronk launch opencode unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT --host 192.168.1.10:11435`}</code></pre>
          </div>

          <div className="card" id="session">
            <h3>Isolated OpenCode Session</h3>
            <p>The existing Kronk server remains independently managed and continues running when OpenCode exits. Its normal <code>model_config.yaml</code> resolution and AutoTune behavior apply.</p>
            <p>OpenCode receives only the selected model and runs with temporary workspace and XDG config, data, cache, and state directories. Existing user and project OpenCode configuration is neither loaded nor modified.</p>
            <p>Kronk prints the temporary environment path before OpenCode starts and confirms its removal after OpenCode exits. In an interactive terminal, Kronk warns that OpenCode may show a blank screen for up to 20 seconds while it initializes and waits for Enter before handing over the terminal.</p>
          </div>

          <div className="card" id="install">
            <h3>OpenCode Installation</h3>
            <p>If OpenCode is absent, Kronk displays the platform-specific install command and <a href="https://opencode.ai/docs/">OpenCode documentation</a>, then asks permission. Installation occurs only after a yes response.</p>
          </div>

          <div className="card" id="uninstall">
            <h3>Uninstall</h3>
            <pre className="code-block"><code>kronk launch opencode uninstall</code></pre>
            <p>This delegates to the official <code>opencode uninstall</code> flow, which previews what it will remove and asks for confirmation. See the <a href="https://opencode.ai/docs/cli/#uninstall">OpenCode uninstall documentation</a>.</p>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# Launch an isolated coding session
kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT

# Pass all arguments after -- directly to OpenCode
kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT -- --help

# Use a Kronk server on another host
kronk launch opencode mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT --host 192.168.1.10:11435

# Uninstall OpenCode through its official workflow
kronk launch opencode uninstall`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section"><a href="#usage" className="doc-index-header">Usage</a></div>
            <div className="doc-index-section"><a href="#flags" className="doc-index-header">Flags</a></div>
            <div className="doc-index-section"><a href="#prerequisites" className="doc-index-header">Prerequisites</a></div>
            <div className="doc-index-section"><a href="#session" className="doc-index-header">Isolated Session</a></div>
            <div className="doc-index-section"><a href="#install" className="doc-index-header">Installation</a></div>
            <div className="doc-index-section"><a href="#uninstall" className="doc-index-header">Uninstall</a></div>
            <div className="doc-index-section"><a href="#examples" className="doc-index-header">Examples</a></div>
          </div>
        </nav>
      </div>
    </div>
  );
}
