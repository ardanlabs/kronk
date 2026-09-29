export default function DocsAPIDecision() {
  return (
    <div>
      <div className="page-header">
        <h2>Decision API</h2>
        <p>Evaluate typed questions against shared state with a supported decision model.</p>
      </div>

      <div className="doc-layout">
        <div className="doc-content">
          <div className="card" id="overview">
            <h3>Overview</h3>
            <p>
              <code>POST /v1/systemone</code> and <code>POST /v1/decide</code> are equivalent.
              Both preserve request object order and return the same typed response.
            </p>
            <p>
              <strong>Authentication:</strong> When authentication is enabled, the token must
              have <code>decision</code> endpoint access.
            </p>
          </div>

          <div className="card" id="decisions">
            <h3>Decisions</h3>
            <div className="doc-section" id="decisions-post">
              <h4><span className="method-post">POST</span> /systemone · /decide</h4>
              <p className="doc-description">
                Evaluate choice, score, and calibrated yes/no questions independently against one shared state value.
              </p>

              <h5>Request Body</h5>
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
                    <td>An installed model recognized as a decision model.</td>
                  </tr>
                  <tr>
                    <td><code>state</code></td>
                    <td>JSON value</td>
                    <td>Yes</td>
                    <td>Shared application state considered by every question.</td>
                  </tr>
                  <tr>
                    <td><code>questions</code></td>
                    <td><code>object</code></td>
                    <td>Yes</td>
                    <td>A nonempty object keyed by caller-defined question IDs.</td>
                  </tr>
                </tbody>
              </table>

              <h5>Question Types</h5>
              <table className="flags-table">
                <thead>
                  <tr>
                    <th>Type</th>
                    <th>Criteria</th>
                    <th>Answer</th>
                  </tr>
                </thead>
                <tbody>
                  <tr>
                    <td><code>choice</code></td>
                    <td>Named option object</td>
                    <td><code>choice</code>, probabilities, and confidence</td>
                  </tr>
                  <tr>
                    <td><code>score</code></td>
                    <td>Ordered array of 2–10 levels</td>
                    <td><code>score</code>, legend, probabilities, and confidence</td>
                  </tr>
                  <tr>
                    <td><code>noul</code></td>
                    <td>Optional false/true descriptions</td>
                    <td><code>noul</code> probability from 0 through 1</td>
                  </tr>
                </tbody>
              </table>

              <h5>Example</h5>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:11435/v1/decide \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0",
    "state": {
      "customer_message": "I was charged twice and need this fixed today.",
      "account_tier": "business"
    },
    "questions": {
      "route": {
        "type": "choice",
        "instructions": "Which team should handle this request?",
        "criteria": {
          "billing": "Payment and invoice issues",
          "support": "Technical product issues",
          "sales": "Plans and new purchases"
        }
      },
      "urgency": {
        "type": "score",
        "instructions": "How urgent is this request?",
        "criteria": ["not urgent", "normal", "urgent", "critical"]
      },
      "requires_human": {
        "type": "noul",
        "instructions": "Should a human review this request?"
      }
    }
  }'`}</code>
              </pre>

              <h5>Response</h5>
              <pre className="code-block">
                <code>{`{
  "model": "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0",
  "answers": {
    "route": {
      "type": "choice",
      "choice": "billing",
      "probabilities": {"billing": 0.96, "support": 0.03, "sales": 0.01},
      "confidence": 0.96
    },
    "urgency": {
      "type": "score",
      "score": 2.7,
      "legend": {"0": "not urgent", "1": "normal", "2": "urgent", "3": "critical"},
      "probabilities": {"0": 0.01, "1": 0.04, "2": 0.2, "3": 0.75},
      "confidence": 0.75
    },
    "requires_human": {"type": "noul", "noul": 0.83}
  },
  "usage": {"input_tokens": 132, "output_tokens": 0}
}`}</code>
              </pre>

              <p>
                Values above are illustrative. Decision readouts consume logits without generating
                text, so <code>output_tokens</code> is zero.
              </p>
            </div>
          </div>
        </div>

        <nav className="doc-sidebar">
          <div className="doc-sidebar-content">
            <div className="doc-index-section">
              <a href="#overview" className="doc-index-header">Overview</a>
            </div>
            <div className="doc-index-section">
              <a href="#decisions" className="doc-index-header">Decisions</a>
              <ul>
                <li><a href="#decisions-post">POST /systemone · /decide</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
