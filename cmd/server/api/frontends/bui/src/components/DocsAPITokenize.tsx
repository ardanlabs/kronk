export default function DocsAPITokenize() {
  return (
    <div>
      <div className="page-header">
        <h2>Tokenize API</h2>
        <p>Get token counts for text input. Optionally apply the model's chat template before tokenizing.</p>
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

          <div className="card" id="tokenize">
            <h3>Tokenize</h3>
            <p>Count the number of tokens for a given text input. Works with any model type.</p>

            <div className="doc-section" id="tokenize-post--tokenize">
              <h4><span className="method-post">POST</span> /tokenize</h4>
              <p className="doc-description">Returns the token count for a text input. Optionally applies the model's chat template before tokenizing to get the actual token count that would be fed to the model.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'tokenize' endpoint access.</p>
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
                    <td>Bearer token for authentication</td>
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
                    <td><code>model</code></td>
                    <td><code>string</code></td>
                    <td>Yes</td>
                    <td>Model ID (e.g., 'Qwen3-8B-Q8_0'). Works with any model type.</td>
                  </tr>
                  <tr>
                    <td><code>input</code></td>
                    <td><code>string</code></td>
                    <td>Yes</td>
                    <td>The text to tokenize.</td>
                  </tr>
                  <tr>
                    <td><code>apply_template</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>If true, wraps the input as a user message and applies the model's chat template before tokenizing. The returned count includes template overhead (role markers, separators, generation prompt). Defaults to false.</td>
                  </tr>
                  <tr>
                    <td><code>add_generation_prompt</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>When apply_template is true, controls whether the assistant role prefix is appended to the prompt. Defaults to true.</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a tokenize object with the token count.</p>
              <h5>Examples</h5>
              <p className="example-label"><strong>Tokenize raw text:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/tokenize \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog"
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Tokenize with chat template applied:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/tokenize \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog",
    "apply_template": true
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Response:</strong></p>
              <pre className="code-block">
                <code>{`{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen3-8B-Q8_0",
  "tokens": 11
}`}</code>
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
              <a href="#tokenize" className="doc-index-header">Tokenize</a>
              <ul>
                <li><a href="#tokenize-post--tokenize">POST /tokenize</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
