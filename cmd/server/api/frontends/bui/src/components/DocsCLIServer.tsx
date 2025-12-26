export default function DocsCLIServer() {
  return (
    <div>
      <div className="page-header">
        <h2>server</h2>
        <p>Manage model server - start, stop, logs.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk server &lt;command&gt; [flags]</code>
            </pre>
          </div>

          <div className="card" id="subcommands">
            <h3>Subcommands</h3>

            <div className="doc-section" id="cmd-start">
              <h4>start</h4>
              <p className="doc-description">Start Kronk model server.</p>
              <pre className="code-block">
                <code>kronk server start [flags]</code>
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
                    <td><code>-d, --detach</code></td>
                    <td>Run server in the background</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Start the server in foreground
kronk server start

# Start the server in background
kronk server start -d

# View all server environment settings
kronk server start --help`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-stop">
              <h4>stop</h4>
              <p className="doc-description">Stop the Kronk model server by sending SIGTERM.</p>
              <pre className="code-block">
                <code>kronk server stop</code>
              </pre>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Stop the server
kronk server stop`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-logs">
              <h4>logs</h4>
              <p className="doc-description">Stream the Kronk model server logs (tail -f).</p>
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
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#usage" className="doc-index-header">Usage</a>
            </div>
            <div className="doc-index-section">
              <a href="#subcommands" className="doc-index-header">Subcommands</a>
              <ul>
                <li><a href="#cmd-start">start</a></li>
                <li><a href="#cmd-stop">stop</a></li>
                <li><a href="#cmd-logs">logs</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
