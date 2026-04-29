export default function DocsCLICatalog() {
  return (
    <div>
      <div className="page-header">
        <h2>catalog</h2>
        <p>Browse and manage the model catalog (list, show, remove).</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk catalog &lt;command&gt; [flags]</code>
            </pre>
            <p>
              The catalog is the curated set of HuggingFace models the system
              knows how to download. Entries are stored in <code>catalog.yaml</code>
              {' '}under <code>~/.kronk/catalog/</code> and seeded from an
              embedded default on first run. The catalog is local and personal —
              there is no remote pull or update.
            </p>
          </div>

          <div className="card" id="subcommands">
            <h3>Subcommands</h3>

            <div className="doc-section" id="cmd-list">
              <h4>list</h4>
              <p className="doc-description">List catalog entries.</p>
              <pre className="code-block">
                <code>kronk catalog list [flags]</code>
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
                    <td>Run without the model server (direct file access)</td>
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
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# List every catalog entry
kronk catalog list

# List with local mode (no server required)
kronk catalog list --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-show">
              <h4>show</h4>
              <p className="doc-description">Display detailed information about a catalog entry.</p>
              <pre className="code-block">
                <code>kronk catalog show &lt;CATALOG_ID&gt; [flags]</code>
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
                    <td>Run without the model server (direct file access)</td>
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
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Show full details for a single entry
kronk catalog show unsloth/Qwen3-8B-GGUF

# Show with local mode
kronk catalog show unsloth/Qwen3-8B-GGUF --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-remove">
              <h4>remove</h4>
              <p className="doc-description">Remove a catalog entry, its GGUF cache, and any downloaded files.</p>
              <pre className="code-block">
                <code>kronk catalog remove &lt;CATALOG_ID&gt; [flags]</code>
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
                    <td>Run without the model server (direct file access)</td>
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
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Remove an entry plus its downloaded files
kronk catalog remove unsloth/Qwen3-8B-GGUF

# Remove with local mode
kronk catalog remove unsloth/Qwen3-8B-GGUF --local`}</code>
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
                <li><a href="#cmd-list">list</a></li>
                <li><a href="#cmd-show">show</a></li>
                <li><a href="#cmd-remove">remove</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
