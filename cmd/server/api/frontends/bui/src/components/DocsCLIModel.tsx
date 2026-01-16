export default function DocsCLIModel() {
  return (
    <div>
      <div className="page-header">
        <h2>model</h2>
        <p>Manage models - list, pull, remove, show, and check running models.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="usage">
            <h3>Usage</h3>
            <pre className="code-block">
              <code>kronk model &lt;command&gt; [flags]</code>
            </pre>
          </div>

          <div className="card" id="subcommands">
            <h3>Subcommands</h3>

            <div className="doc-section" id="cmd-index">
              <h4>index</h4>
              <p className="doc-description">Rebuild the model index for fast model access.</p>
              <pre className="code-block">
                <code>kronk model index [flags]</code>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Rebuild the model index
kronk model index

# Rebuild with local mode
kronk model index --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-list">
              <h4>list</h4>
              <p className="doc-description">List models.</p>
              <pre className="code-block">
                <code>kronk model list [flags]</code>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# List all models
kronk model list

# List with local mode
kronk model list --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-ps">
              <h4>ps</h4>
              <p className="doc-description">List running models.</p>
              <pre className="code-block">
                <code>kronk model ps</code>
              </pre>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# List running models
kronk model ps`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-pull">
              <h4>pull</h4>
              <p className="doc-description">Pull a model from the web.</p>
              <pre className="code-block">
                <code>kronk model pull &lt;MODEL_URL&gt; [MMPROJ_URL] [flags]</code>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Pull a model from a URL
kronk model pull https://huggingface.co/.../model.gguf

# Pull with local mode
kronk model pull https://huggingface.co/.../model.gguf --local

# Pull a vision model with mmproj file
kronk model pull <MODEL_URL> <MMPROJ_URL>`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-remove">
              <h4>remove</h4>
              <p className="doc-description">Remove a model.</p>
              <pre className="code-block">
                <code>kronk model remove &lt;MODEL_NAME&gt; [flags]</code>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Remove a model
kronk model remove llama-3.2-1b-q4

# Remove with local mode
kronk model remove llama-3.2-1b-q4 --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-show">
              <h4>show</h4>
              <p className="doc-description">Show information for a model.</p>
              <pre className="code-block">
                <code>kronk model show &lt;MODEL_NAME&gt; [flags]</code>
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
                    <td>localhost:8080</td>
                    <td>IP Address for the kronk server (web mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_BASE_PATH</code></td>
                    <td>$HOME/kronk</td>
                    <td>Base path for kronk data directories (local mode)</td>
                  </tr>
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Show model information
kronk model show llama-3.2-1b-q4

# Show with local mode
kronk model show llama-3.2-1b-q4 --local`}</code>
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
                <li><a href="#cmd-index">index</a></li>
                <li><a href="#cmd-list">list</a></li>
                <li><a href="#cmd-ps">ps</a></li>
                <li><a href="#cmd-pull">pull</a></li>
                <li><a href="#cmd-remove">remove</a></li>
                <li><a href="#cmd-show">show</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
