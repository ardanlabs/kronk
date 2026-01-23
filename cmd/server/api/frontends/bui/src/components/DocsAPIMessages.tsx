export default function DocsAPIMessages() {
  return (
    <div>
      <div className="page-header">
        <h2>Messages API</h2>
        <p>Generate messages using language models. Compatible with the Anthropic Messages API.</p>
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

          <div className="card" id="messages">
            <h3>Messages</h3>
            <p>Create messages with language models using the Anthropic Messages API format.</p>

            <div className="doc-section" id="messages-post--messages">
              <h4><span className="method-post">POST</span> /messages</h4>
              <p className="doc-description">Create a message. Supports streaming responses with Server-Sent Events using Anthropic's event format.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'messages' endpoint access.</p>
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
                  <tr>
                    <td><code>anthropic-version</code></td>
                    <td>No</td>
                    <td>API version (optional)</td>
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
                    <td><code>messages</code></td>
                    <td><code>array</code></td>
                    <td>Yes</td>
                    <td>Array of message objects with role (user/assistant) and content</td>
                  </tr>
                  <tr>
                    <td><code>max_tokens</code></td>
                    <td><code>integer</code></td>
                    <td>Yes</td>
                    <td>Maximum number of tokens to generate</td>
                  </tr>
                  <tr>
                    <td><code>system</code></td>
                    <td><code>string|array</code></td>
                    <td>No</td>
                    <td>System prompt as string or array of content blocks</td>
                  </tr>
                  <tr>
                    <td><code>stream</code></td>
                    <td><code>boolean</code></td>
                    <td>No</td>
                    <td>Enable streaming responses (default: false)</td>
                  </tr>
                  <tr>
                    <td><code>tools</code></td>
                    <td><code>array</code></td>
                    <td>No</td>
                    <td>List of tools the model can use</td>
                  </tr>
                  <tr>
                    <td><code>temperature</code></td>
                    <td><code>number</code></td>
                    <td>No</td>
                    <td>Sampling temperature (0-1)</td>
                  </tr>
                  <tr>
                    <td><code>top_p</code></td>
                    <td><code>number</code></td>
                    <td>No</td>
                    <td>Nucleus sampling parameter</td>
                  </tr>
                  <tr>
                    <td><code>top_k</code></td>
                    <td><code>integer</code></td>
                    <td>No</td>
                    <td>Top-k sampling parameter</td>
                  </tr>
                  <tr>
                    <td><code>stop_sequences</code></td>
                    <td><code>array</code></td>
                    <td>No</td>
                    <td>Sequences where the API will stop generating</td>
                  </tr>
                </tbody>
              </table>
              <h5>Response</h5>
              <p>Returns a message object, or streams Server-Sent Events if stream=true. Response includes anthropic-request-id header.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Basic message:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>With system prompt:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "system": "You are a helpful assistant.",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Streaming response:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a haiku about coding"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Multi-turn conversation:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "What about 2+3?"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Vision with image URL (requires vision model):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is in this image?"},
          {"type": "image", "source": {"type": "url", "url": "https://example.com/image.jpg"}}
        ]
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Vision with base64 image (requires vision model):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image"},
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/jpeg",
              "data": "/9j/4AAQ..."
            }
          }
        ]
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Tool calling:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"}
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "City name"
            }
          },
          "required": ["location"]
        }
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Tool result (continue conversation after tool call):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/messages \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"},
      {
        "role": "assistant",
        "content": [
          {
            "type": "tool_use",
            "id": "call_xyz789",
            "name": "get_weather",
            "input": {"location": "Paris"}
          }
        ]
      },
      {
        "role": "user",
        "content": [
          {
            "type": "tool_result",
            "tool_use_id": "call_xyz789",
            "content": "Sunny, 22°C"
          }
        ]
      }
    ],
    "tools": [
      {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "input_schema": {
          "type": "object",
          "properties": {
            "location": {"type": "string"}
          },
          "required": ["location"]
        }
      }
    ]
  }'`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="response-formats">
            <h3>Response Formats</h3>
            <p>The Messages API returns different formats for streaming and non-streaming responses.</p>

            <div className="doc-section" id="response-formats--non-streaming-response">
              <h4>Non-Streaming Response</h4>
              <p className="doc-description">For non-streaming requests (stream=false or omitted), returns a complete message object.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "text",
      "text": "Hello! I'm doing well, thank you for asking. How can I help you today?"
    }
  ],
  "model": "qwen3-8b-q8_0",
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 12,
    "output_tokens": 18
  }
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-formats--tool-use-response">
              <h4>Tool Use Response</h4>
              <p className="doc-description">When the model calls a tool, the content includes tool_use blocks with the tool call details.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "id": "msg_abc123",
  "type": "message",
  "role": "assistant",
  "content": [
    {
      "type": "tool_use",
      "id": "call_xyz789",
      "name": "get_weather",
      "input": {
        "location": "Paris"
      }
    }
  ],
  "model": "qwen3-8b-q8_0",
  "stop_reason": "tool_use",
  "usage": {
    "input_tokens": 50,
    "output_tokens": 25
  }
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-formats--streaming-events">
              <h4>Streaming Events</h4>
              <p className="doc-description">For streaming requests (stream=true), the API returns Server-Sent Events with different event types following Anthropic's streaming format.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`event: message_start
data: {"type":"message_start","message":{"id":"msg_abc123","type":"message","role":"assistant","content":[],"model":"qwen3-8b-q8_0","stop_reason":null,"usage":{"input_tokens":12,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"!"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":18}}

event: message_stop
data: {"type":"message_stop"}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-formats--streaming-tool-calls">
              <h4>Streaming Tool Calls</h4>
              <p className="doc-description">When streaming tool calls, input_json_delta events provide incremental JSON for tool arguments.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_xyz789","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\\"location\\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\\"Paris\\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":25}}

event: message_stop
data: {"type":"message_stop"}`}</code>
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
              <a href="#messages" className="doc-index-header">Messages</a>
              <ul>
                <li><a href="#messages-post--messages">POST /messages</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#response-formats" className="doc-index-header">Response Formats</a>
              <ul>
                <li><a href="#response-formats--non-streaming-response">Non-Streaming Response</a></li>
                <li><a href="#response-formats--tool-use-response">Tool Use Response</a></li>
                <li><a href="#response-formats--streaming-events">Streaming Events</a></li>
                <li><a href="#response-formats--streaming-tool-calls">Streaming Tool Calls</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
