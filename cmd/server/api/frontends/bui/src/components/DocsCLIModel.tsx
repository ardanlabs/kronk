export default function DocsCLIModel() {
  return (
    <div>
      <div className="page-header">
        <h2>model</h2>
        <p>Manage local models (index, list, pull, remove, resolve, show, ps).</p>
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
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/.kronk/models</td>
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
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/.kronk/models</td>
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
                    <td>localhost:11435</td>
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
              <p className="doc-description">Pull a model from the web. The projection file is located automatically.</p>
              <pre className="code-block">
                <code>kronk model pull &lt;SOURCE&gt; [flags]</code>
              </pre>
              <p>The source may be:</p>
              <ul>
                <li>A bare model id: <code>Qwen3-0.6B-Q8_0</code> (resolved via the provider list)</li>
                <li>A canonical id: <code>unsloth/Qwen3-0.6B-Q8_0</code> (skips provider walk)</li>
                <li>A full HuggingFace URL: <code>https://huggingface.co/org/repo/resolve/main/model.gguf</code></li>
                <li>A short form: <code>org/repo/model.gguf</code></li>
                <li>A shorthand: <code>owner/repo:Q4_K_M</code> (auto-resolves files via the HuggingFace API)</li>
                <li>With hf.co prefix: <code>hf.co/owner/repo:Q4_K_M</code></li>
                <li>With revision: <code>owner/repo:Q4_K_M@revision</code></li>
              </ul>
              <p>
                By default the projection file (when applicable) is located
                automatically. Bare and canonical ids consult{' '}
                <code>~/.kronk/catalog.yaml</code> first, then walk the configured
                provider list (<code>unsloth</code>, <code>ggml-org</code>,{' '}
                <code>bartowski</code>, ...) and persist the resolution. Multi-file
                (split) models are downloaded in full when the resolver expands
                them.
              </p>
              <p>
                Use <code>--proj &lt;URL&gt;</code> to pin a specific projection
                file. The flag takes a fully qualified HuggingFace URL and forces
                the explicit-URL workflow:
              </p>
              <ul>
                <li>With an id source the resolver is consulted only to expand split shards; the supplied projection URL replaces the resolver's choice.</li>
                <li>With a URL source the model file at that URL is paired directly with the supplied projection URL — no resolver lookup.</li>
              </ul>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Flag</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>--proj &lt;URL&gt;</code></td>
                    <td>Fully qualified projection (mmproj) URL to pin (skips auto-resolution)</td>
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
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/.kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Pull by canonical HuggingFace id (projection auto-resolved)
kronk model pull unsloth/Qwen3-8B-GGUF

# Pull with shorthand (auto-resolves files)
kronk model pull unsloth/Qwen3-8B-GGUF:Q4_K_M

# Pull a vision model and pin a specific projection file
kronk model pull <MODEL_URL> --proj <MMPROJ_URL>

# Pull with local mode
kronk model pull unsloth/Qwen3-8B-GGUF --local`}</code>
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
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/.kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Remove a model
kronk model remove unsloth/Qwen3-8B-GGUF

# Remove with local mode
kronk model remove unsloth/Qwen3-8B-GGUF --local`}</code>
              </pre>
            </div>

            <div className="doc-section" id="cmd-resolve">
              <h4>resolve</h4>
              <p className="doc-description">
                Resolve a model id to a provider, repo, files and download URLs.
                Useful for inspecting how the catalog and provider list will be
                walked before issuing a <code>pull</code>.
              </p>
              <pre className="code-block">
                <code>kronk model resolve &lt;MODEL_ID&gt; [flags]</code>
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
                    <td><code>--refresh</code></td>
                    <td>Bypass the resolver-file cache and force a HuggingFace lookup</td>
                  </tr>
                  <tr>
                    <td><code>--base-path &lt;string&gt;</code></td>
                    <td>Base path for kronk data (models, libraries, catalog, model_config) — persistent global flag</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Resolve a bare model id
kronk model resolve Qwen3-0.6B-Q8_0

# Resolve with cache bypass (force HF lookup)
kronk model resolve Qwen3-0.6B-Q8_0 --refresh`}</code>
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
                  <tr>
                    <td><code>KRONK_MODELS</code></td>
                    <td>$HOME/.kronk/models</td>
                    <td>The path to the models directory (local mode)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`# Show model information
kronk model show unsloth/Qwen3-8B-GGUF

# Show with local mode
kronk model show unsloth/Qwen3-8B-GGUF --local`}</code>
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
                <li><a href="#cmd-resolve">resolve</a></li>
                <li><a href="#cmd-show">show</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
