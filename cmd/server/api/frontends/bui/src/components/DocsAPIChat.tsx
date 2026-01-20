export default function DocsAPIChat() {
  return (
    <div>
      <div className="page-header">
        <h2>Chat Completions API</h2>
        <p>Generate chat completions using language models. Compatible with the OpenAI Chat Completions API.</p>
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

          <div className="card" id="chat-completions">
            <h3>Chat Completions</h3>
            <p>Create chat completions with language models.</p>

            <div className="doc-section" id="chat-completions-post--chat-completions">
              <h4><span className="method-post">POST</span> /chat/completions</h4>
              <p className="doc-description">Create a chat completion. Supports streaming responses.</p>
              <p><strong>Authentication:</strong> Required when auth is enabled. Token must have 'chat-completions' endpoint access.</p>
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
                    <td>Model ID to use for completion (e.g., 'qwen3-8b-q8_0')</td>
                  </tr>
                  <tr>
                    <td><code>messages</code></td>
                    <td><code>array</code></td>
                    <td>Yes</td>
                    <td>Array of message objects. See Message Formats section below for supported formats.</td>
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
                    <td>Array of tool definitions for function calling. See Tool Definitions section below.</td>
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
              <p>Returns a chat completion object, or streams Server-Sent Events if stream=true.</p>
              <h5>Example</h5>
              <p className="example-label"><strong>Simple text message:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "Hello, how are you?"
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Multi-turn conversation:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "And what is that multiplied by 3?"}
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Vision - image from URL (requires vision model):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is in this image?"},
          {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
        ]
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Vision - base64 encoded image (requires vision model):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2.5-vl-3b-instruct-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "Describe this image"},
          {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}}
        ]
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Audio - base64 encoded audio (requires audio model):</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen2-audio-7b-q8_0",
    "messages": [
      {
        "role": "user",
        "content": [
          {"type": "text", "text": "What is being said in this audio?"},
          {"type": "input_audio", "input_audio": {"data": "UklGRi...", "format": "wav"}}
        ]
      }
    ]
  }'`}</code>
              </pre>
              <p className="example-label"><strong>Tool/Function calling - define tools and let the model call them:</strong></p>
              <pre className="code-block">
                <code>{`curl -X POST http://localhost:8080/v1/chat/completions \\
  -H "Authorization: Bearer $KRONK_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {"role": "user", "content": "What is the weather in Tokyo?"}
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
              "location": {
                "type": "string",
                "description": "The location to get the weather for, e.g. San Francisco, CA"
              }
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

          <div className="card" id="response-formats">
            <h3>Response Formats</h3>
            <p>The response format differs between streaming and non-streaming requests.</p>

            <div className="doc-section" id="response-formats--non-streaming-response">
              <h4>Non-Streaming Response</h4>
              <p className="doc-description">For non-streaming requests (stream=false or omitted), the response uses the 'message' field in each choice. The 'delta' field is empty.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "qwen3-8b-q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "Hello! I'm doing well, thank you for asking.",
        "reasoning": ""
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "reasoning_tokens": 0,
    "completion_tokens": 12,
    "output_tokens": 12,
    "total_tokens": 37,
    "tokens_per_second": 85.5
  }
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="response-formats--streaming-response">
              <h4>Streaming Response</h4>
              <p className="doc-description">For streaming requests (stream=true), the response uses the 'delta' field in each choice. Multiple chunks are sent as Server-Sent Events, with incremental content in each delta.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`// Each chunk contains partial content in the delta field
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" How"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" are you?"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{...}}
data: [DONE]`}</code>
              </pre>
            </div>
          </div>

          <div className="card" id="message-formats">
            <h3>Message Formats</h3>
            <p>The messages array supports several formats depending on the content type and model capabilities.</p>

            <div className="doc-section" id="message-formats--text-messages">
              <h4>Text Messages</h4>
              <p className="doc-description">Simple text content with role (system, user, or assistant) and content string.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "role": "system",
  "content": "You are a helpful assistant."
}

{
  "role": "user",
  "content": "Hello, how are you?"
}

{
  "role": "assistant",
  "content": "I'm doing well, thank you!"
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="message-formats--multi-part-content-(vision)">
              <h4>Multi-part Content (Vision)</h4>
              <p className="doc-description">For vision models, content can be an array with text and image parts. Images can be URLs or base64-encoded data URIs.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "role": "user",
  "content": [
    {"type": "text", "text": "What is in this image?"},
    {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}}
  ]
}

// Base64 encoded image
{
  "role": "user",
  "content": [
    {"type": "text", "text": "Describe this image"},
    {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}}
  ]
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="message-formats--audio-content">
              <h4>Audio Content</h4>
              <p className="doc-description">For audio models, content can include audio data as base64-encoded input with format specification.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "role": "user",
  "content": [
    {"type": "text", "text": "What is being said?"},
    {"type": "input_audio", "input_audio": {"data": "UklGRi...", "format": "wav"}}
  ]
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="message-formats--tool-definitions">
              <h4>Tool Definitions</h4>
              <p className="doc-description">Tools are defined in the 'tools' array field of the request (not in messages). Each tool specifies a function with name, description, and parameters schema.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`// Tools are defined at the request level
{
  "model": "qwen3-8b-q8_0",
  "messages": [...],
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "get_weather",
        "description": "Get the current weather for a location",
        "parameters": {
          "type": "object",
          "properties": {
            "location": {
              "type": "string",
              "description": "The location to get the weather for, e.g. San Francisco, CA"
            }
          },
          "required": ["location"]
        }
      }
    }
  ]
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="message-formats--tool-call-response-(non-streaming)">
              <h4>Tool Call Response (Non-Streaming)</h4>
              <p className="doc-description">For non-streaming requests (stream=false), when the model calls a tool, the response uses the 'message' field with 'tool_calls' array. The finish_reason is 'tool_calls'.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`{
  "id": "chatcmpl-abc123",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "qwen3-8b-q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "",
        "tool_calls": [
          {
            "id": "call_xyz789",
            "index": 0,
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\\"location\\":\\"Tokyo\\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {
    "prompt_tokens": 50,
    "completion_tokens": 25,
    "total_tokens": 75
  }
}`}</code>
              </pre>
            </div>

            <div className="doc-section" id="message-formats--tool-call-response-(streaming)">
              <h4>Tool Call Response (Streaming)</h4>
              <p className="doc-description">For streaming requests (stream=true), tool calls are returned in the 'delta' field. Each chunk contains partial tool call data that should be accumulated.</p>
              <h5>Example</h5>
              <pre className="code-block">
                <code>{`// Streaming chunks with tool calls use delta instead of message
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_xyz789","index":0,"type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\\"location\\":"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\\"Tokyo\\"}"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{...}}
data: [DONE]`}</code>
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
              <a href="#chat-completions" className="doc-index-header">Chat Completions</a>
              <ul>
                <li><a href="#chat-completions-post--chat-completions">POST /chat/completions</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#response-formats" className="doc-index-header">Response Formats</a>
              <ul>
                <li><a href="#response-formats--non-streaming-response">Non-Streaming Response</a></li>
                <li><a href="#response-formats--streaming-response">Streaming Response</a></li>
              </ul>
            </div>
            <div className="doc-index-section">
              <a href="#message-formats" className="doc-index-header">Message Formats</a>
              <ul>
                <li><a href="#message-formats--text-messages">Text Messages</a></li>
                <li><a href="#message-formats--multi-part-content-(vision)">Multi-part Content (Vision)</a></li>
                <li><a href="#message-formats--audio-content">Audio Content</a></li>
                <li><a href="#message-formats--tool-definitions">Tool Definitions</a></li>
                <li><a href="#message-formats--tool-call-response-(non-streaming)">Tool Call Response (Non-Streaming)</a></li>
                <li><a href="#message-formats--tool-call-response-(streaming)">Tool Call Response (Streaming)</a></li>
              </ul>
            </div>
          </div>
        </nav>
      </div>
    </div>
  );
}
