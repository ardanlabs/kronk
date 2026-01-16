export default function DocsAPIResponses() {
  return (
    <div>
      <div className="page-header">
        <h2>Responses API</h2>
        <p>Generate responses using language models. Compatible with the OpenAI Responses API.</p>
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

          <div className="card" id="responses">
            <h3>Responses</h3>
            <p>Create responses with language models using the Responses API format.</p>

            <div className="doc-section" id="responses-post--responses">
              <h4><span className="method-post">POST</span> /responses</h4>
              <p className="doc-description">Create a response. Supports streaming responses with Server-Sent Events.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'responses' endpoint access.</p>
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
                    <td>ID of the model to use</td>
                  </tr>
                  <tr>
                    <td><code>input</code></td>
                    <td><code>array</code></td>
                    <td>Yes</td>
                    <td>Array of input messages (same format as chat messages)</td>
                  </tr>
                  <tr>
                    <td><code>stream</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Enable streaming responses (default: false)</td>
                  </tr>
                  <tr>
                    <td><code>instructions</code></td>
                    <td><code>string</code></td>
                    <td>No</td>
                    <td>System instructions for the model</td>
                  </tr>
                  <tr>
                    <td><code>tools</code></td>
                    <td><code>array</code></td>
                    <td>No</td>
                    <td>List of tools the model can use</td>
                  </tr>
                  <tr>
                    <td><code>tool_choice</code></td>
                    <td><code>string</code></td>
                    <td>No</td>
                    <td>How the model should use tools: auto, none, or required</td>
                  </tr>
                  <tr>
                    <td><code>parallel_tool_calls</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Allow parallel tool calls (default: true)</td>
                  </tr>
                  <tr>
                    <td><code>store</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Whether to store the response (default: true)</td>
                  </tr>
                  <tr>
                    <td><code>truncation</code></td>
                    <td><code>string</code></td>
                    <td>No</td>
                    <td>Truncation strategy: auto or disabled (default: disabled)</td>
                  </tr>
                  <tr>
                    <td><code>temperature</code></td>
                    <td><code>float32</code></td>
                    <td>No</td>
                    <td>Controls randomness of output (default: 0.8)</td>
                  </tr>
                  <tr>
                    <td><code>top_k</code></td>
                    <td><code>int32</code></td>
                    <td>No</td>
                    <td>Limits token pool to K most probable tokens (default: 40)</td>
                  </tr>
                  <tr>
                    <td><code>top_p</code></td>
                    <td><code>float32</code></td>
                    <td>No</td>
                    <td>Nucleus sampling threshold (default: 0.9)</td>
                  </tr>
                  <tr>
                    <td><code>min_p</code></td>
                    <td><code>float32</code></td>
                    <td>No</td>
                    <td>Dynamic sampling threshold (default: 0.0)</td>
                  </tr>
                  <tr>
                    <td><code>max_tokens</code></td>
                    <td><code>int</code></td>
                    <td>No</td>
                    <td>Maximum output tokens (default: 1024)</td>
                  </tr>
                  <tr>
                    <td><code>enable_thinking</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Enable model thinking for non-GPT models (default: true)</td>
                  </tr>
                  <tr>
                    <td><code>reasoning_effort</code></td>
                    <td><code>string</code></td>
                    <td>No</td>
                    <td>Reasoning level for GPT models (default: medium)</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a response object, or streams Server-Sent Events if stream=true.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Basic response:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Streaming response:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Write a short poem about coding"}
    ],
    "stream": true
  }'`}</code>
              </pre>
              <p className="example-label"><strong>With tools:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/responses \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "What is the weather in London?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get the current weather for a location",
          "parameters": {
            "type": "object",
            "properties": {
              "location": {"type": "string", "description": "City name"}
            },
            "required": ["location"]
          }
        }
      }
    ]
  }'`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="response-format">
            <h3>Response Format</h3>
            <p>The Responses API returns a structured response object with output items.</p>

            <div className="doc-section" id="response-format--response-object">
              <h4>Response Object</h4>
              <p className="doc-description">The response object contains metadata, output items, and usage information.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "id": "resp_abc123",
  "object": "response",
  "created_at": 1234567890,
  "status": "completed",
  "model": "qwen3-8b-q8_0",
  "output": [
    {
      "type": "message",
      "id": "msg_xyz789",
      "status": "completed",
      "role": "assistant",
      "content": [
        {
          "type": "output_text",
          "text": "Hello! I'm doing well, thank you for asking.",
          "annotations": []
        }
      ]
    }
  ],
  "usage": {
    "input_tokens": 12,
    "output_tokens": 15,
    "total_tokens": 27
  }
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-format--streaming-events">
              <h4>Streaming Events</h4>
              <p className="doc-description">When stream=true, the API returns Server-Sent Events with different event types.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`event: response.created
data: {"type":"response.created","response":{...}}

event: response.in_progress
data: {"type":"response.in_progress","response":{...}}

event: response.output_item.added
data: {"type":"response.output_item.added","item":{...}}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"Hello"}

event: response.output_text.done
data: {"type":"response.output_text.done","text":"Hello! How are you?"}

event: response.completed
data: {"type":"response.completed","response":{...}}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-format--function-call-output">
              <h4>Function Call Output</h4>
              <p className="doc-description">When the model calls a tool, the output contains a function_call item instead of a message.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "output": [
    {
      "type": "function_call",
      "id": "fc_abc123",
      "call_id": "call_xyz789",
      "name": "get_weather",
      "arguments": "{\\"location\\":\\"London\\"}",
      "status": "completed"
    }
  ]
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
              <a href="#responses" className="doc-index-header">Responses</a>
              <ul>
                <li><a href="#responses-post--responses">POST /responses</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#response-format" className="doc-index-header">Response Format</a>
              <ul>
                <li><a href="#response-format--response-object">Response Object</a></li>
                <li><a href="#response-format--streaming-events">Streaming Events</a></li>
                <li><a href="#response-format--function-call-output">Function Call Output</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
