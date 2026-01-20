export default function DocsAPIRerank() {
  return (
    <div>
      <div className="page-header">
        <h2>Rerank API</h2>
        <p>Rerank documents by relevance to a query. Used for semantic search result ordering.</p>
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

          <div className="card" id="reranking">
            <h3>Reranking</h3>
            <p>Score and reorder documents by relevance to a query.</p>

            <div className="doc-section" id="reranking-post--rerank">
              <h4><span className="method-post">POST</span> /rerank</h4>
              <p className="doc-description">Rerank documents by their relevance to a query. The model must support reranking.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'rerank' endpoint access.</p>
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
                    <td>Reranker model ID (e.g., 'bge-reranker-v2-m3-Q8_0')</td>
                  </tr>
                  <tr>
                    <td><code>query</code></td>
                    <td><code>string</code></td>
                    <td>Yes</td>
                    <td>The query to rank documents against.</td>
                  </tr>
                  <tr>
                    <td><code>documents</code></td>
                    <td><code>array</code></td>
                    <td>Yes</td>
                    <td>Array of document strings to rank.</td>
                  </tr>
                  <tr>
                    <td><code>top_n</code></td>
                    <td><code>integer</code></td>
                    <td>No</td>
                    <td>Return only the top N results. Defaults to all documents.</td>
                  </tr>
                  <tr>
                    <td><code>return_documents</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Include document text in results. Defaults to false.</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a list of reranked results with index and relevance_score, sorted by score descending.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Rerank documents for a query:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/rerank \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks."
    ],
    "top_n": 2
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
              <a href="#reranking" className="doc-index-header">Reranking</a>
              <ul>
                <li><a href="#reranking-post--rerank">POST /rerank</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
