export default function DocsAPITools() {
  return (
    <div>
      <div className="page-header">
        <h2>Tools API</h2>
        <p>Manage libraries, models, catalog, and security. These endpoints handle server administration tasks.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="overview">
            <h3>Overview</h3>
            <p>All endpoints are prefixed with <code>/v1</code>. Base URL: <code>http://localhost:8080</code></p>
            <h4>Authentication</h4>
            <p>When authentication is enabled, include the token in the Authorization header:</p>
            <pre className="code-block">
              <code>Authorization: Bearer YOUR_TOKEN</code>
            </pre>
          </div>

          <div className="card" id="libs">
            <h3>Libs</h3>
            <p>Manage llama.cpp libraries installation and updates.</p>

            <div className="doc-section" id="libs-get--libs">
              <h4><span className="method-get">GET</span> /libs</h4>
              <p className="doc-description">Get information about installed llama.cpp libraries.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns version information including arch, os, processor, latest version, and current version.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Get library information:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/libs`}</code>
              </pre>
            </div>

            <div className="doc-section" id="libs-post--libs-pull">
              <h4><span className="method-post">POST</span> /libs/pull</h4>
              <p className="doc-description">Download and install the latest llama.cpp libraries. Returns streaming progress updates.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Streams download progress as Server-Sent Events.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Pull latest libraries:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/libs/pull`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="models">
            <h3>Models</h3>
            <p>Manage models - list, pull, show, and remove models from the server.</p>

            <div className="doc-section" id="models-get--models">
              <h4><span className="method-get">GET</span> /models</h4>
              <p className="doc-description">List all available models on the server.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a list of model objects with id, owned_by, model_family, size, and modified fields.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>List all models:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/models`}</code>
              </pre>
            </div>

            <div className="doc-section" id="models-get--models-model">
              <h4><span className="method-get">GET</span> /models/&#123;model&#125;</h4>
              <p className="doc-description">Show detailed information about a specific model.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns model details including metadata, capabilities, and configuration.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Show model details:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/models/qwen3-8b-q8_0`}</code>
              </pre>
            </div>

            <div className="doc-section" id="models-get--models-ps">
              <h4><span className="method-get">GET</span> /models/ps</h4>
              <p className="doc-description">List currently loaded/running models in the cache.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a list of running models with id, owned_by, model_family, size, expires_at, and active_streams.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>List running models:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/models/ps`}</code>
              </pre>
            </div>

            <div className="doc-section" id="models-post--models-index">
              <h4><span className="method-post">POST</span> /models/index</h4>
              <p className="doc-description">Rebuild the model index for fast model access.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns empty response on success.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Rebuild model index:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/models/index`}</code>
              </pre>
            </div>

            <div className="doc-section" id="models-post--models-pull">
              <h4><span className="method-post">POST</span> /models/pull</h4>
              <p className="doc-description">Pull/download a model from a URL. Returns streaming progress updates.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                  <tr>
                    <td><code>Content-Type</code></td>
                    <td>Yes</td>
                    <td>Must be application/json</td>
                  </tr>
                </tbody>
              </table>
              <h5>Request Body</h5>
              <p><code>application/json</code></p>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Field</th>
                    <th>Type</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>model_url</code></td>
                    <td><code>string</code></td>
                    <td>Yes</td>
                    <td>URL to the model GGUF file</td>
                  </tr>
                  <tr>
                    <td><code>proj_url</code></td>
                    <td><code>string</code></td>
                    <td>No</td>
                    <td>URL to the projection file (for vision/audio models)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Streams download progress as Server-Sent Events.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Pull a model from HuggingFace:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/models/pull \\
  -H "Content-Type: application/json" \\
  -d '{
    "model_url": "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf"
  }'`}</code>
              </pre>
            </div>

            <div className="doc-section" id="models-delete--models-model">
              <h4><span className="method-delete">DELETE</span> /models/&#123;model&#125;</h4>
              <p className="doc-description">Remove a model from the server.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns empty response on success.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Remove a model:</strong></p>
              <pre className="code-block">
                <code>{`curl -X DELETE http://localhost:8080/v1/models/qwen3-8b-q8_0`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="catalog">
            <h3>Catalog</h3>
            <p>Browse and pull models from the curated model catalog.</p>

            <div className="doc-section" id="catalog-get--catalog">
              <h4><span className="method-get">GET</span> /catalog</h4>
              <p className="doc-description">List all models available in the catalog.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a list of catalog models with id, category, owned_by, model_family, and capabilities.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>List catalog models:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/catalog`}</code>
              </pre>
            </div>

            <div className="doc-section" id="catalog-get--catalog-filter-filter">
              <h4><span className="method-get">GET</span> /catalog/filter/&#123;filter&#125;</h4>
              <p className="doc-description">List catalog models filtered by category.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a filtered list of catalog models.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Filter catalog by category:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/catalog/filter/embedding`}</code>
              </pre>
            </div>

            <div className="doc-section" id="catalog-get--catalog-model">
              <h4><span className="method-get">GET</span> /catalog/&#123;model&#125;</h4>
              <p className="doc-description">Show detailed information about a catalog model.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns full catalog model details including files, capabilities, and metadata.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Show catalog model details:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/catalog/qwen3-8b-q8_0`}</code>
              </pre>
            </div>

            <div className="doc-section" id="catalog-post--catalog-pull-model">
              <h4><span className="method-post">POST</span> /catalog/pull/&#123;model&#125;</h4>
              <p className="doc-description">Pull a model from the catalog by ID. Returns streaming progress updates.</p>
              <p><strong>Authentication:</strong> Optional when auth is enabled.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>No</td>
                    <td>Bearer token for authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Streams download progress as Server-Sent Events.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Pull a catalog model:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/catalog/pull/qwen3-8b-q8_0`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="security">
            <h3>Security</h3>
            <p>Manage security tokens and private keys for authentication.</p>

            <div className="doc-section" id="security-post--security-token-create">
              <h4><span className="method-post">POST</span> /security/token/create</h4>
              <p className="doc-description">Create a new security token with specified permissions and duration.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                  <tr>
                    <td><code>Content-Type</code></td>
                    <td>Yes</td>
                    <td>Must be application/json</td>
                  </tr>
                </tbody>
              </table>
              <h5>Request Body</h5>
              <p><code>application/json</code></p>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Field</th>
                    <th>Type</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>admin</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Whether the token has admin privileges</td>
                  </tr>
                  <tr>
                    <td><code>duration</code></td>
                    <td><code>duration</code></td>
                    <td>Yes</td>
                    <td>Token validity duration (e.g., '24h', '720h')</td>
                  </tr>
                  <tr>
                    <td><code>endpoints</code></td>
                    <td><code>object</code></td>
                    <td>Yes</td>
                    <td>Map of endpoint names to rate limit configurations</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns the created token string.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Create a token with chat-completions access:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/security/token/create \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "admin": false,
    "duration": "24h",
    "endpoints": {
      "chat-completions": {"limit": 1000, "window": "day"},
      "embeddings": {"limit": 0, "window": ""}
    }
  }'`}</code>
              </pre>
            </div>

            <div className="doc-section" id="security-get--security-keys">
              <h4><span className="method-get">GET</span> /security/keys</h4>
              <p className="doc-description">List all private keys in the system.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a list of keys with id and created timestamp.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>List all keys:</strong></p>
              <pre className="code-block">
                <code>{`curl -X GET http://localhost:8080/v1/security/keys \\
  -H "Authorization: Bearer $KRONK_TOKEN"`}</code>
              </pre>
            </div>

            <div className="doc-section" id="security-post--security-keys-add">
              <h4><span className="method-post">POST</span> /security/keys/add</h4>
              <p className="doc-description">Create a new private key and add it to the keystore.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns empty response on success.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Add a new key:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/security/keys/add \\
  -H "Authorization: Bearer $KRONK_TOKEN"`}</code>
              </pre>
            </div>

            <div className="doc-section" id="security-post--security-keys-remove-keyid">
              <h4><span className="method-post">POST</span> /security/keys/remove/&#123;keyid&#125;</h4>
              <p className="doc-description">Remove a private key from the keystore by its ID.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Admin token required.</p>
              <h5>Headers</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Header</th>
                    <th>Required</th>
                    <th>Description</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>Authorization</code></td>
                    <td>Yes</td>
                    <td>Bearer token for admin authentication</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns empty response on success.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Remove a key:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/security/keys/remove/abc123 \\
  -H "Authorization: Bearer $KRONK_TOKEN"`}</code>
              </pre>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#overview" className="doc-index-header">Overview</a>
            </div>
            <div className="doc-index-section">
              <a href="#libs" className="doc-index-header">Libs</a>
              <ul>
                <li><a href="#libs-get--libs">GET /libs</a></li>
                <li><a href="#libs-post--libs-pull">POST /libs/pull</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#models" className="doc-index-header">Models</a>
              <ul>
                <li><a href="#models-get--models">GET /models</a></li>
                <li><a href="#models-get--models-model">GET /models/&#123;model&#125;</a></li>
                <li><a href="#models-get--models-ps">GET /models/ps</a></li>
                <li><a href="#models-post--models-index">POST /models/index</a></li>
                <li><a href="#models-post--models-pull">POST /models/pull</a></li>
                <li><a href="#models-delete--models-model">DELETE /models/&#123;model&#125;</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#catalog" className="doc-index-header">Catalog</a>
              <ul>
                <li><a href="#catalog-get--catalog">GET /catalog</a></li>
                <li><a href="#catalog-get--catalog-filter-filter">GET /catalog/filter/&#123;filter&#125;</a></li>
                <li><a href="#catalog-get--catalog-model">GET /catalog/&#123;model&#125;</a></li>
                <li><a href="#catalog-post--catalog-pull-model">POST /catalog/pull/&#123;model&#125;</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#security" className="doc-index-header">Security</a>
              <ul>
                <li><a href="#security-post--security-token-create">POST /security/token/create</a></li>
                <li><a href="#security-get--security-keys">GET /security/keys</a></li>
                <li><a href="#security-post--security-keys-add">POST /security/keys/add</a></li>
                <li><a href="#security-post--security-keys-remove-keyid">POST /security/keys/remove/&#123;keyid&#125;</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
