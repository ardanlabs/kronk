export default function DocsAPIEmbeddings() {
  return (
    <div>
      <div className="page-header">
        <h2>Embeddings API</h2>
        <p>Generate vector embeddings for text. Compatible with the OpenAI Embeddings API.</p>
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

          <div className="card" id="embeddings">
            <h3>Embeddings</h3>
            <p>Create vector embeddings for semantic search and similarity.</p>

            <div className="doc-section" id="embeddings-post--embeddings">
              <h4><span className="method-post">POST</span> /embeddings</h4>
              <p className="doc-description">Create embeddings for the given input text. The model must support embedding generation.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'embeddings' endpoint access.</p>
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
                    <td>Embedding model ID (e.g., 'embeddinggemma-300m-qat-Q8_0')</td>
                  </tr>
                  <tr>
                    <td><code>input</code></td>
                    <td><code>string|array</code></td>
                    <td>Yes</td>
                    <td>Text to generate embeddings for. Can be a string or array of strings.</td>
                  </tr>
                  <tr>
                    <td><code>dimensions</code></td>
                    <td><code>integer</code></td>
                    <td>No</td>
                    <td>Reduce output to first N dimensions (for Matryoshka models). Must be &lt;= model's native dimensions.</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns an embedding object with vector data.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Generate embeddings for text:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/embeddings \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "Why is the sky blue?"
  }'`}</code>
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
              <a href="#embeddings" className="doc-index-header">Embeddings</a>
              <ul>
                <li><a href="#embeddings-post--embeddings">POST /embeddings</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
