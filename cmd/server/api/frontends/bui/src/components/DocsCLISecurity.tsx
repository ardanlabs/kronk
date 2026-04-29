export default function DocsCLISecurity() {
  return (
    <div>
      <div className="page-header">
        <h2>security</h2>
        <p>Manage API security (keys and tokens).</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk security &lt;command&gt; [flags]</code>
            </pre>
            <p>
              Provides JWT-based access control for the Kronk Model Server.
              It manages two types of credentials:
            </p>
            <ul>
              <li>
                <strong>Private keys</strong> — used to sign JWT tokens. Each
                key has a unique ID and is used for token issuance. Keys can
                be created, listed, and revoked.
              </li>
              <li>
                <strong>JWT tokens</strong> — short-lived credentials issued
                by private keys. Tokens authenticate API requests and can
                include custom claims for fine-grained authorization.
              </li>
            </ul>
            <p>
              All security commands require an admin-level token in{' '}
              <code>KRONK_TOKEN</code> before execution.
            </p>
          </div>

          <div className="card" id="cmd-key">
            <h3>key — Manage private keys</h3>
            <pre className="code-block">
              <code>kronk security key &lt;command&gt; [flags]</code>
            </pre>

            <div className="doc-section" id="cmd-key-create">
              <h4>create</h4>
              <p className="doc-description">Create a new private key and add it to the keystore.</p>
              <pre className="code-block">
                <code>kronk security key create [flags]</code>
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
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
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
                    <td>Admin token (required)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_WEB_API_HOST</code></td>
                    <td>localhost:11435</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Create a new private key
export KRONK_TOKEN=<admin-token>
kronk security key create`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-key-delete">
              <h4>delete</h4>
              <p className="doc-description">Delete a private key by its key ID (file name without extension).</p>
              <pre className="code-block">
                <code>kronk security key delete --keyid &lt;KEY_ID&gt; [flags]</code>
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
                    <td><code>--keyid &lt;string&gt;</code></td>
                    <td>The key ID to delete (required)</td>
                  </tr>
                  <tr>
                    <td><code>--local</code></td>
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
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
                    <td>Admin token (required)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_WEB_API_HOST</code></td>
                    <td>localhost:11435</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Delete a private key
export KRONK_TOKEN=<admin-token>
kronk security key delete --keyid abc123`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-key-list">
              <h4>list</h4>
              <p className="doc-description">List all private keys in the system.</p>
              <pre className="code-block">
                <code>kronk security key list [flags]</code>
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
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
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
                    <td>Admin token (required)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_WEB_API_HOST</code></td>
                    <td>localhost:11435</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# List all private keys
export KRONK_TOKEN=<admin-token>
kronk security key list`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="cmd-token">
            <h3>token — Manage JWT tokens</h3>
            <pre className="code-block">
              <code>kronk security token &lt;command&gt; [flags]</code>
            </pre>

            <div className="doc-section" id="cmd-token-create">
              <h4>create</h4>
              <p className="doc-description">Create a security token.</p>
              <pre className="code-block">
                <code>kronk security token create [flags]</code>
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
                    <td><code>--duration &lt;duration&gt;</code></td>
                    <td>Token duration (e.g. <code>1h</code>, <code>24h</code>, <code>720h</code>)</td>
                  </tr>
                  <tr>
                    <td><code>--endpoints &lt;list&gt;</code></td>
                    <td>Endpoints with optional rate limits (e.g. <code>chat-completions:1000/day</code>)</td>
                  </tr>
                  <tr>
                    <td><code>--local</code></td>
                    <td>Run without the model server</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
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
                    <td>Admin token (required)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_WEB_API_HOST</code></td>
                    <td>localhost:11435</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Create a token with 24 hour duration
export KRONK_TOKEN=<admin-token>
kronk security token create --duration=24h --endpoints=chat-completions,embeddings

# Create a token with rate limits
kronk security token create --duration=720h --endpoints="chat-completions:1000/day,embeddings:unlimited"`}</code>
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
              <a href="#cmd-key" className="doc-index-header">key</a>
              <ul>
                <li><a href="#cmd-key-create">create</a></li>
                <li><a href="#cmd-key-delete">delete</a></li>
                <li><a href="#cmd-key-list">list</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#cmd-token" className="doc-index-header">token</a>
              <ul>
                <li><a href="#cmd-token-create">create</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
