export default function DocsCLILibs() {
  return (
    <div>
      <div className="page-header">
        <h2>libs</h2>
        <p>Install or upgrade llama.cpp libraries.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk libs [flags]</code>
            </pre>
          </div>

          <div className="card" id="subcommands">
            <h3>Subcommands</h3>

            <div className="doc-section" id="cmd-(default)">
              <h4>(default)</h4>
              <p className="doc-description">
                Install or upgrade llama.cpp libraries. By default the
                command installs the <strong>well-known default
                version</strong> of llama.cpp baked into the SDK; pass{' '}
                <code>--upgrade</code> to track the latest published
                release instead.
              </p>
              <pre className="code-block">
                <code>kronk libs [flags]</code>
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
                    <td><code>--local</code></td>
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--upgrade</code></td>
                    <td>Track the latest llama.cpp release instead of the well-known default version (default: <code>false</code>)</td>
                  </tr>
                  <tr>
                    <td><code>--version &lt;string&gt;</code></td>
                    <td>Download a specific llama.cpp version instead of the default (e.g. <code>b5540</code>). See <a href="https://github.com/ggml-org/llama.cpp/releases" target="_blank" rel="noopener noreferrer">available releases</a>. An explicit version overrides both the default and <code>--upgrade</code>.</td>
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
                    <td><code>KRONK_TOKEN</code></td>
                    <td></td>
                    <td>Authentication token for the kronk server (required when auth enabled)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_WEB_API_HOST</code></td>
                    <td>localhost:11435</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_ARCH</code></td>
                    <td>runtime.GOARCH</td>
                    <td>The architecture to install (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_LIB_PATH</code></td>
                    <td>$HOME/kronk/libraries</td>
                    <td>The path to the libraries directory (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_OS</code></td>
                    <td>runtime.GOOS</td>
                    <td>The operating system to install (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_PROCESSOR</code></td>
                    <td>cpu</td>
                    <td>Options: cpu, cuda, metal, rocm, vulkan (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Install the default version using the server
kronk libs

# Install the default version locally
kronk libs --local

# Track and install the latest llama.cpp release
kronk libs --local --upgrade

# Install a specific version
kronk libs --version b5540
kronk libs --local --version b5540

# Install with Metal support on macOS
KRONK_PROCESSOR=metal kronk libs --local`}</code>
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
                <li><a href="#cmd-(default)">(default)</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
