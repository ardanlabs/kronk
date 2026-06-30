export default function DocsCLIDevices() {
  return (
    <div>
      <div className="page-header">
        <h2>devices</h2>
        <p>List available compute devices.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk devices</code>
            </pre>
            <p>
              List all available compute devices that can be used for model
              inference. Each row reports the device index, name, and type
              (CPU or GPU, with the backend in parentheses), followed by
              whether GPU offload is supported on this host.
            </p>
            <p>
              The device names shown here can be used with the{' '}
              <code>--devices</code> flag of the <code>run</code> command, or
              the <code>devices</code> field in model configuration.
            </p>
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
                  <td>$HOME/.kronk</td>
                  <td>Base path for kronk data directories</td>
                </tr>
                <tr>
                  <td><code>KRONK_LIB_PATH</code></td>
                  <td>$HOME/.kronk/libraries</td>
                  <td>Library directory path. Device detection uses the installed llama.cpp bundle.</td>
                </tr>
              </tbody>
            </table>
          </div>

          <div className="card" id="examples">
            <h3>Examples</h3>
            <pre className="code-block">
              <code>{`# List all devices
kronk devices`}</code>
            </pre>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#flags" className="doc-index-header">Flags</a>
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
