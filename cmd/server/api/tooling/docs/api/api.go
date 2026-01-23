// Package api provides documentation tooling for the Web API docs.
package api

import (
	"fmt"
	"os"
	"strings"
)

type endpoint struct {
	Method      string
	Path        string
	Description string
	Auth        string
	Headers     []header
	RequestBody *requestBody
	Response    *response
	Examples    []example
}

type header struct {
	Name        string
	Description string
	Required    bool
}

type requestBody struct {
	ContentType string
	Fields      []field
}

type field struct {
	Name        string
	Type        string
	Description string
	Required    bool
}

type response struct {
	ContentType string
	Description string
	Fields      []field
}

type example struct {
	Description string
	Code        string
}

type endpointGroup struct {
	Name        string
	Description string
	Endpoints   []endpoint
}

type apiDoc struct {
	Name        string
	Description string
	Groups      []endpointGroup
	Filename    string
	Component   string
}

var outputDir = "cmd/server/api/frontends/bui/src/components"

func Run() error {
	docs := []apiDoc{
		chatDoc(),
		messagesDoc(),
		responsesDoc(),
		embeddingsDoc(),
		rerankDoc(),
		toolsDoc(),
	}

	for _, doc := range docs {
		tsx := generateAPITSX(doc)

		outputPath := outputDir + "/" + doc.Filename
		if err := os.WriteFile(outputPath, []byte(tsx), 0644); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}

		fmt.Printf("Generated %s\n", outputPath)
	}

	return nil
}

func generateAPITSX(doc apiDoc) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("export default function %s() {\n", doc.Component))
	b.WriteString("  return (\n")
	b.WriteString("    <div>\n")

	b.WriteString("      <div className=\"page-header\">\n")
	b.WriteString(fmt.Sprintf("        <h2>%s</h2>\n", doc.Name))
	b.WriteString(fmt.Sprintf("        <p>%s</p>\n", escapeJSX(doc.Description)))
	b.WriteString("      </div>\n\n")

	b.WriteString("      <div className=\"doc-layout\">\n")
	b.WriteString("        <div className=\"doc-content\">\n")

	b.WriteString("          <div className=\"card\" id=\"overview\">\n")
	b.WriteString("            <h3>Overview</h3>\n")
	b.WriteString("            <p>All endpoints are prefixed with <code>/v1</code>. Base URL: <code>http://localhost:8080</code></p>\n")
	b.WriteString("            <h4>Authentication</h4>\n")
	b.WriteString("            <p>When authentication is enabled, include the token in the Authorization header:</p>\n")
	b.WriteString("            <pre className=\"code-block\">\n")
	b.WriteString("              <code>Authorization: Bearer YOUR_TOKEN</code>\n")
	b.WriteString("            </pre>\n")
	b.WriteString("          </div>\n")

	for _, group := range doc.Groups {
		anchor := toAnchor(group.Name)

		b.WriteString(fmt.Sprintf("\n          <div className=\"card\" id=\"%s\">\n", anchor))
		b.WriteString(fmt.Sprintf("            <h3>%s</h3>\n", group.Name))
		b.WriteString(fmt.Sprintf("            <p>%s</p>\n", escapeJSX(group.Description)))

		for _, ep := range group.Endpoints {
			epAnchor := toAnchor(group.Name + "-" + ep.Method + "-" + ep.Path)
			b.WriteString(fmt.Sprintf("\n            <div className=\"doc-section\" id=\"%s\">\n", epAnchor))

			switch ep.Method != "" {
			case true:
				b.WriteString(fmt.Sprintf("              <h4><span className=\"method-%s\">%s</span> %s</h4>\n",
					strings.ToLower(ep.Method), ep.Method, escapeJSX(ep.Path)))
			case false:
				b.WriteString(fmt.Sprintf("              <h4>%s</h4>\n", escapeJSX(ep.Path)))
			}

			b.WriteString(fmt.Sprintf("              <p className=\"doc-description\">%s</p>\n", escapeJSX(ep.Description)))

			if ep.Auth != "" {
				b.WriteString(fmt.Sprintf("              <p><strong>Authentication:</strong> %s</p>\n", escapeJSX(ep.Auth)))
			}

			if len(ep.Headers) > 0 {
				b.WriteString("              <h5>Headers</h5>\n")
				b.WriteString("              <table className=\"flags-table\">\n")
				b.WriteString("                <thead>\n")
				b.WriteString("                  <tr>\n")
				b.WriteString("                    <th>Header</th>\n")
				b.WriteString("                    <th>Required</th>\n")
				b.WriteString("                    <th>Description</th>\n")
				b.WriteString("                  </tr>\n")
				b.WriteString("                </thead>\n")
				b.WriteString("                <tbody>\n")

				for _, h := range ep.Headers {
					required := "No"
					if h.Required {
						required = "Yes"
					}
					b.WriteString("                  <tr>\n")
					b.WriteString(fmt.Sprintf("                    <td><code>%s</code></td>\n", escapeJSX(h.Name)))
					b.WriteString(fmt.Sprintf("                    <td>%s</td>\n", required))
					b.WriteString(fmt.Sprintf("                    <td>%s</td>\n", escapeJSX(h.Description)))
					b.WriteString("                  </tr>\n")
				}

				b.WriteString("                </tbody>\n")
				b.WriteString("              </table>\n")
			}

			if ep.RequestBody != nil && len(ep.RequestBody.Fields) > 0 {
				b.WriteString("              <h5>Request Body</h5>\n")
				b.WriteString(fmt.Sprintf("              <p><code>%s</code></p>\n", ep.RequestBody.ContentType))
				b.WriteString("              <table className=\"flags-table\">\n")
				b.WriteString("                <thead>\n")
				b.WriteString("                  <tr>\n")
				b.WriteString("                    <th>Field</th>\n")
				b.WriteString("                    <th>Type</th>\n")
				b.WriteString("                    <th>Required</th>\n")
				b.WriteString("                    <th>Description</th>\n")
				b.WriteString("                  </tr>\n")
				b.WriteString("                </thead>\n")
				b.WriteString("                <tbody>\n")

				for _, f := range ep.RequestBody.Fields {
					required := "No"
					if f.Required {
						required = "Yes"
					}
					b.WriteString("                  <tr>\n")
					b.WriteString(fmt.Sprintf("                    <td><code>%s</code></td>\n", escapeJSX(f.Name)))
					b.WriteString(fmt.Sprintf("                    <td><code>%s</code></td>\n", escapeJSX(f.Type)))
					b.WriteString(fmt.Sprintf("                    <td>%s</td>\n", required))
					b.WriteString(fmt.Sprintf("                    <td>%s</td>\n", escapeJSX(f.Description)))
					b.WriteString("                  </tr>\n")
				}

				b.WriteString("                </tbody>\n")
				b.WriteString("              </table>\n")
			}

			if ep.Response != nil {
				b.WriteString("              <h5>Response</h5>\n")
				b.WriteString(fmt.Sprintf("              <p>%s</p>\n", escapeJSX(ep.Response.Description)))
			}

			if len(ep.Examples) > 0 {
				b.WriteString("              <h5>Example</h5>\n")
				for _, ex := range ep.Examples {
					if ex.Description != "" {
						b.WriteString(fmt.Sprintf("              <p className=\"example-label\"><strong>%s</strong></p>\n", escapeJSX(ex.Description)))
					}
					b.WriteString("              <pre className=\"code-block\">\n")
					b.WriteString(fmt.Sprintf("                <code>{`%s`}</code>\n", escapeTemplateLiteral(ex.Code)))
					b.WriteString("              </pre>\n")
				}
			}

			b.WriteString("            </div>\n")
		}

		b.WriteString("          </div>\n")
	}

	b.WriteString("        </div>\n")

	b.WriteString("\n        <nav className=\"doc-sidebar\">\n")
	b.WriteString("          <div className=\"doc-sidebar-content\">\n")

	b.WriteString("            <div className=\"doc-index-section\">\n")
	b.WriteString("              <a href=\"#overview\" className=\"doc-index-header\">Overview</a>\n")
	b.WriteString("            </div>\n")

	for _, group := range doc.Groups {
		anchor := toAnchor(group.Name)
		b.WriteString("            <div className=\"doc-index-section\">\n")
		b.WriteString(fmt.Sprintf("              <a href=\"#%s\" className=\"doc-index-header\">%s</a>\n", anchor, group.Name))
		b.WriteString("              <ul>\n")

		for _, ep := range group.Endpoints {
			epAnchor := toAnchor(group.Name + "-" + ep.Method + "-" + ep.Path)
			var label string
			switch ep.Method != "" {
			case true:
				label = ep.Method + " " + escapeJSX(ep.Path)
			case false:
				label = escapeJSX(ep.Path)
			}
			b.WriteString(fmt.Sprintf("                <li><a href=\"#%s\">%s</a></li>\n", epAnchor, label))
		}

		b.WriteString("              </ul>\n")
		b.WriteString("            </div>\n")
	}

	b.WriteString("          </div>\n")
	b.WriteString("        </nav>\n")

	b.WriteString("      </div>\n")
	b.WriteString("    </div>\n")
	b.WriteString("  );\n")
	b.WriteString("}\n")

	return b.String()
}

func toAnchor(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "{", "")
	s = strings.ReplaceAll(s, "}", "")

	return s
}

func escapeJSX(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "{", "&#123;")
	s = strings.ReplaceAll(s, "}", "&#125;")

	return s
}

func escapeTemplateLiteral(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "${", "\\${")

	return s
}

// =============================================================================

func chatCompletionFields() []field {
	baseFields := []field{
		{Name: "model", Type: "string", Required: true, Description: "Model ID to use for completion (e.g., 'qwen3-8b-q8_0')"},
		{Name: "messages", Type: "array", Required: true, Description: "Array of message objects. See Message Formats section below for supported formats."},
		{Name: "stream", Type: "boolean", Required: false, Description: "Enable streaming responses (default: false)"},
		{Name: "tools", Type: "array", Required: false, Description: "Array of tool definitions for function calling. See Tool Definitions section below."},
	}

	paramFields := paramsToFields()

	return append(baseFields, paramFields...)
}

func chatCompletionExamples() []example {
	return []example{
		{
			Description: "Simple text message:",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Multi-turn conversation:",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "stream": true,
    "model": "qwen3-8b-q8_0",
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "And what is that multiplied by 3?"}
    ]
  }'`,
		},
		{
			Description: "Vision - image from URL (requires vision model):",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Vision - base64 encoded image (requires vision model):",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Audio - base64 encoded audio (requires audio model):",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Tool/Function calling - define tools and let the model call them:",
			Code: `curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
	}
}

func chatDoc() apiDoc {
	return apiDoc{
		Name:        "Chat Completions API",
		Description: "Generate chat completions using language models. Compatible with the OpenAI Chat Completions API.",
		Filename:    "DocsAPIChat.tsx",
		Component:   "DocsAPIChat",
		Groups: []endpointGroup{
			{
				Name:        "Chat Completions",
				Description: "Create chat completions with language models.",
				Endpoints: []endpoint{
					{
						Method:      "POST",
						Path:        "/chat/completions",
						Description: "Create a chat completion. Supports streaming responses.",
						Auth:        "Required when auth is enabled. Token must have 'chat-completions' endpoint access.",
						Headers: []header{
							{Name: "Authorization", Description: "Bearer token for authentication", Required: true},
							{Name: "Content-Type", Description: "Must be application/json", Required: true},
						},
						RequestBody: &requestBody{
							ContentType: "application/json",
							Fields:      chatCompletionFields(),
						},
						Response: &response{
							ContentType: "application/json or text/event-stream",
							Description: "Returns a chat completion object, or streams Server-Sent Events if stream=true.",
						},
						Examples: chatCompletionExamples(),
					},
				},
			},
			chatResponseFormatsGroup(),
			messageFormatsGroup(),
		},
	}
}

func chatResponseFormatsGroup() endpointGroup {
	return endpointGroup{
		Name:        "Response Formats",
		Description: "The response format differs between streaming and non-streaming requests.",
		Endpoints: []endpoint{
			{
				Method:      "",
				Path:        "Non-Streaming Response",
				Description: "For non-streaming requests (stream=false or omitted), the response uses the 'message' field in each choice. The 'delta' field is empty.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Streaming Response",
				Description: "For streaming requests (stream=true), the response uses the 'delta' field in each choice. Multiple chunks are sent as Server-Sent Events, with incremental content in each delta.",
				Examples: []example{
					{
						Code: `// Each chunk contains partial content in the delta field
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" How"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"content":" are you?"},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{...}}
data: [DONE]`,
					},
				},
			},
		},
	}
}

func messagesFields() []field {
	fields := []field{
		{Name: "model", Type: "string", Required: true, Description: "ID of the model to use"},
		{Name: "messages", Type: "array", Required: true, Description: "Array of message objects with role (user/assistant) and content"},
		{Name: "max_tokens", Type: "integer", Required: true, Description: "Maximum number of tokens to generate"},
		{Name: "system", Type: "string|array", Required: false, Description: "System prompt as string or array of content blocks"},
		{Name: "stream", Type: "boolean", Required: false, Description: "Enable streaming responses (default: false)"},
		{Name: "tools", Type: "array", Required: false, Description: "List of tools the model can use"},
		{Name: "temperature", Type: "number", Required: false, Description: "Sampling temperature (0-1)"},
		{Name: "top_p", Type: "number", Required: false, Description: "Nucleus sampling parameter"},
		{Name: "top_k", Type: "integer", Required: false, Description: "Top-k sampling parameter"},
		{Name: "stop_sequences", Type: "array", Required: false, Description: "Sequences where the API will stop generating"},
	}

	return fields
}

func messagesExamples() []example {
	return []example{
		{
			Description: "Basic message:",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`,
		},
		{
			Description: "With system prompt:",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "system": "You are a helpful assistant.",
    "messages": [
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'`,
		},
		{
			Description: "Streaming response:",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "stream": true,
    "messages": [
      {"role": "user", "content": "Write a haiku about coding"}
    ]
  }'`,
		},
		{
			Description: "Multi-turn conversation:",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "What is 2+2?"},
      {"role": "assistant", "content": "2+2 equals 4."},
      {"role": "user", "content": "What about 2+3?"}
    ]
  }'`,
		},
		{
			Description: "Vision with image URL (requires vision model):",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Vision with base64 image (requires vision model):",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Tool calling:",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
		{
			Description: "Tool result (continue conversation after tool call):",
			Code: `curl -X POST http://localhost:8080/v1/messages \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
	}
}

func messagesFormatsGroup() endpointGroup {
	return endpointGroup{
		Name:        "Response Formats",
		Description: "The Messages API returns different formats for streaming and non-streaming responses.",
		Endpoints: []endpoint{
			{
				Method:      "",
				Path:        "Non-Streaming Response",
				Description: "For non-streaming requests (stream=false or omitted), returns a complete message object.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Tool Use Response",
				Description: "When the model calls a tool, the content includes tool_use blocks with the tool call details.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Streaming Events",
				Description: "For streaming requests (stream=true), the API returns Server-Sent Events with different event types following Anthropic's streaming format.",
				Examples: []example{
					{
						Code: `event: message_start
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
data: {"type":"message_stop"}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Streaming Tool Calls",
				Description: "When streaming tool calls, input_json_delta events provide incremental JSON for tool arguments.",
				Examples: []example{
					{
						Code: `event: message_start
data: {"type":"message_start","message":{...}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_xyz789","name":"get_weather","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"location\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":25}}

event: message_stop
data: {"type":"message_stop"}`,
					},
				},
			},
		},
	}
}

func messagesDoc() apiDoc {
	return apiDoc{
		Name:        "Messages API",
		Description: "Generate messages using language models. Compatible with the Anthropic Messages API.",
		Filename:    "DocsAPIMessages.tsx",
		Component:   "DocsAPIMessages",
		Groups: []endpointGroup{
			{
				Name:        "Messages",
				Description: "Create messages with language models using the Anthropic Messages API format.",
				Endpoints: []endpoint{
					{
						Method:      "POST",
						Path:        "/messages",
						Description: "Create a message. Supports streaming responses with Server-Sent Events using Anthropic's event format.",
						Auth:        "Required when auth is enabled. Token must have 'messages' endpoint access.",
						Headers: []header{
							{Name: "Authorization", Description: "Bearer token for authentication", Required: true},
							{Name: "Content-Type", Description: "Must be application/json", Required: true},
							{Name: "anthropic-version", Description: "API version (optional)", Required: false},
						},
						RequestBody: &requestBody{
							ContentType: "application/json",
							Fields:      messagesFields(),
						},
						Response: &response{
							ContentType: "application/json or text/event-stream",
							Description: "Returns a message object, or streams Server-Sent Events if stream=true. Response includes anthropic-request-id header.",
						},
						Examples: messagesExamples(),
					},
				},
			},
			messagesFormatsGroup(),
		},
	}
}

func responsesDoc() apiDoc {
	return apiDoc{
		Name:        "Responses API",
		Description: "Generate responses using language models. Compatible with the OpenAI Responses API.",
		Filename:    "DocsAPIResponses.tsx",
		Component:   "DocsAPIResponses",
		Groups: []endpointGroup{
			{
				Name:        "Responses",
				Description: "Create responses with language models using the Responses API format.",
				Endpoints: []endpoint{
					{
						Method:      "POST",
						Path:        "/responses",
						Description: "Create a response. Supports streaming responses with Server-Sent Events.",
						Auth:        "Required when auth is enabled. Token must have 'responses' endpoint access.",
						Headers: []header{
							{Name: "Authorization", Description: "Bearer token for authentication", Required: true},
							{Name: "Content-Type", Description: "Must be application/json", Required: true},
						},
						RequestBody: &requestBody{
							ContentType: "application/json",
							Fields:      responsesFields(),
						},
						Response: &response{
							ContentType: "application/json or text/event-stream",
							Description: "Returns a response object, or streams Server-Sent Events if stream=true.",
						},
						Examples: responsesExamples(),
					},
				},
			},
			responsesFormatsGroup(),
		},
	}
}

func responsesFields() []field {
	fields := []field{
		{Name: "model", Type: "string", Required: true, Description: "ID of the model to use"},
		{Name: "input", Type: "array", Required: true, Description: "Array of input messages (same format as chat messages)"},
		{Name: "stream", Type: "boolean", Required: false, Description: "Enable streaming responses (default: false)"},
		{Name: "instructions", Type: "string", Required: false, Description: "System instructions for the model"},
		{Name: "tools", Type: "array", Required: false, Description: "List of tools the model can use"},
		{Name: "tool_choice", Type: "string", Required: false, Description: "How the model should use tools: auto, none, or required"},
		{Name: "parallel_tool_calls", Type: "boolean", Required: false, Description: "Allow parallel tool calls (default: true)"},
		{Name: "store", Type: "boolean", Required: false, Description: "Whether to store the response (default: true)"},
		{Name: "truncation", Type: "string", Required: false, Description: "Truncation strategy: auto or disabled (default: disabled)"},
	}

	fields = append(fields, paramsToFields()...)

	return fields
}

func responsesExamples() []example {
	return []example{
		{
			Description: "Basic response:",
			Code: `curl -X POST http://localhost:8080/v1/responses \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Hello, how are you?"}
    ]
  }'`,
		},
		{
			Description: "Streaming response:",
			Code: `curl -X POST http://localhost:8080/v1/responses \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "qwen3-8b-q8_0",
    "input": [
      {"role": "user", "content": "Write a short poem about coding"}
    ],
    "stream": true
  }'`,
		},
		{
			Description: "With tools:",
			Code: `curl -X POST http://localhost:8080/v1/responses \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
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
  }'`,
		},
	}
}

func responsesFormatsGroup() endpointGroup {
	return endpointGroup{
		Name:        "Response Format",
		Description: "The Responses API returns a structured response object with output items.",
		Endpoints: []endpoint{
			{
				Method:      "",
				Path:        "Response Object",
				Description: "The response object contains metadata, output items, and usage information.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Streaming Events",
				Description: "When stream=true, the API returns Server-Sent Events with different event types.",
				Examples: []example{
					{
						Code: `event: response.created
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
data: {"type":"response.completed","response":{...}}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Function Call Output",
				Description: "When the model calls a tool, the output contains a function_call item instead of a message.",
				Examples: []example{
					{
						Code: `{
  "output": [
    {
      "type": "function_call",
      "id": "fc_abc123",
      "call_id": "call_xyz789",
      "name": "get_weather",
      "arguments": "{\"location\":\"London\"}",
      "status": "completed"
    }
  ]
}`,
					},
				},
			},
		},
	}
}

func messageFormatsGroup() endpointGroup {
	return endpointGroup{
		Name:        "Message Formats",
		Description: "The messages array supports several formats depending on the content type and model capabilities.",
		Endpoints: []endpoint{
			{
				Method:      "",
				Path:        "Text Messages",
				Description: "Simple text content with role (system, user, or assistant) and content string.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Multi-part Content (Vision)",
				Description: "For vision models, content can be an array with text and image parts. Images can be URLs or base64-encoded data URIs.",
				Examples: []example{
					{
						Code: `{
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Audio Content",
				Description: "For audio models, content can include audio data as base64-encoded input with format specification.",
				Examples: []example{
					{
						Code: `{
  "role": "user",
  "content": [
    {"type": "text", "text": "What is being said?"},
    {"type": "input_audio", "input_audio": {"data": "UklGRi...", "format": "wav"}}
  ]
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Tool Definitions",
				Description: "Tools are defined in the 'tools' array field of the request (not in messages). Each tool specifies a function with name, description, and parameters schema.",
				Examples: []example{
					{
						Code: `// Tools are defined at the request level
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Tool Call Response (Non-Streaming)",
				Description: "For non-streaming requests (stream=false), when the model calls a tool, the response uses the 'message' field with 'tool_calls' array. The finish_reason is 'tool_calls'.",
				Examples: []example{
					{
						Code: `{
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
              "arguments": "{\"location\":\"Tokyo\"}"
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
}`,
					},
				},
			},
			{
				Method:      "",
				Path:        "Tool Call Response (Streaming)",
				Description: "For streaming requests (stream=true), tool calls are returned in the 'delta' field. Each chunk contains partial tool call data that should be accumulated.",
				Examples: []example{
					{
						Code: `// Streaming chunks with tool calls use delta instead of message
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"id":"call_xyz789","index":0,"type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Tokyo\"}"}}]},"finish_reason":""}]}
data: {"id":"chatcmpl-abc123","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{...}}
data: [DONE]`,
					},
				},
			},
		},
	}
}

func embeddingsDoc() apiDoc {
	return apiDoc{
		Name:        "Embeddings API",
		Description: "Generate vector embeddings for text. Compatible with the OpenAI Embeddings API.",
		Filename:    "DocsAPIEmbeddings.tsx",
		Component:   "DocsAPIEmbeddings",
		Groups: []endpointGroup{
			{
				Name:        "Embeddings",
				Description: "Create vector embeddings for semantic search and similarity.",
				Endpoints: []endpoint{
					{
						Method:      "POST",
						Path:        "/embeddings",
						Description: "Create embeddings for the given input text. The model must support embedding generation.",
						Auth:        "Required when auth is enabled. Token must have 'embeddings' endpoint access.",
						Headers: []header{
							{Name: "Authorization", Description: "Bearer token for authentication", Required: true},
							{Name: "Content-Type", Description: "Must be application/json", Required: true},
						},
						RequestBody: &requestBody{
							ContentType: "application/json",
							Fields: []field{
								{Name: "model", Type: "string", Required: true, Description: "Embedding model ID (e.g., 'embeddinggemma-300m-qat-Q8_0')"},
								{Name: "input", Type: "string|array", Required: true, Description: "Text to generate embeddings for. Can be a string or array of strings."},
								{Name: "dimensions", Type: "integer", Required: false, Description: "Reduce output to first N dimensions (for Matryoshka models). Must be <= model's native dimensions."},
							},
						},
						Response: &response{
							ContentType: "application/json",
							Description: "Returns an embedding object with vector data.",
						},
						Examples: []example{
							{
								Description: "Generate embeddings for text:",
								Code: `curl -X POST http://localhost:8080/v1/embeddings \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "Why is the sky blue?"
  }'`,
							},
						},
					},
				},
			},
		},
	}
}

func rerankDoc() apiDoc {
	return apiDoc{
		Name:        "Rerank API",
		Description: "Rerank documents by relevance to a query. Used for semantic search result ordering.",
		Filename:    "DocsAPIRerank.tsx",
		Component:   "DocsAPIRerank",
		Groups: []endpointGroup{
			{
				Name:        "Reranking",
				Description: "Score and reorder documents by relevance to a query.",
				Endpoints: []endpoint{
					{
						Method:      "POST",
						Path:        "/rerank",
						Description: "Rerank documents by their relevance to a query. The model must support reranking.",
						Auth:        "Required when auth is enabled. Token must have 'rerank' endpoint access.",
						Headers: []header{
							{Name: "Authorization", Description: "Bearer token for authentication", Required: true},
							{Name: "Content-Type", Description: "Must be application/json", Required: true},
						},
						RequestBody: &requestBody{
							ContentType: "application/json",
							Fields: []field{
								{Name: "model", Type: "string", Required: true, Description: "Reranker model ID (e.g., 'bge-reranker-v2-m3-Q8_0')"},
								{Name: "query", Type: "string", Required: true, Description: "The query to rank documents against."},
								{Name: "documents", Type: "array", Required: true, Description: "Array of document strings to rank."},
								{Name: "top_n", Type: "integer", Required: false, Description: "Return only the top N results. Defaults to all documents."},
								{Name: "return_documents", Type: "boolean", Required: false, Description: "Include document text in results. Defaults to false."},
							},
						},
						Response: &response{
							ContentType: "application/json",
							Description: "Returns a list of reranked results with index and relevance_score, sorted by score descending.",
						},
						Examples: []example{
							{
								Description: "Rerank documents for a query:",
								Code: `curl -X POST http://localhost:8080/v1/rerank \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks."
    ],
    "top_n": 2
  }'`,
							},
						},
					},
				},
			},
		},
	}
}

func toolsDoc() apiDoc {
	return apiDoc{
		Name:        "Tools API",
		Description: "Manage libraries, models, catalog, and security. These endpoints handle server administration tasks.",
		Filename:    "DocsAPITools.tsx",
		Component:   "DocsAPITools",
		Groups: []endpointGroup{
			libsEndpoints(),
			modelsEndpoints(),
			catalogEndpoints(),
			securityEndpoints(),
		},
	}
}

func libsEndpoints() endpointGroup {
	return endpointGroup{
		Name:        "Libs",
		Description: "Manage llama.cpp libraries installation and updates.",
		Endpoints: []endpoint{
			{
				Method:      "GET",
				Path:        "/libs",
				Description: "Get information about installed llama.cpp libraries.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns version information including arch, os, processor, latest version, and current version.",
				},
				Examples: []example{
					{
						Description: "Get library information:",
						Code:        `curl -X GET http://localhost:8080/v1/libs`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/libs/pull",
				Description: "Download and install the latest llama.cpp libraries. Returns streaming progress updates.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "text/event-stream",
					Description: "Streams download progress as Server-Sent Events.",
				},
				Examples: []example{
					{
						Description: "Pull latest libraries:",
						Code:        `curl -X POST http://localhost:8080/v1/libs/pull`,
					},
				},
			},
		},
	}
}

func modelsEndpoints() endpointGroup {
	return endpointGroup{
		Name:        "Models",
		Description: "Manage models - list, pull, show, and remove models from the server.",
		Endpoints: []endpoint{
			{
				Method:      "GET",
				Path:        "/models",
				Description: "List all available models on the server.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns a list of model objects with id, owned_by, model_family, size, and modified fields.",
				},
				Examples: []example{
					{
						Description: "List all models:",
						Code:        `curl -X GET http://localhost:8080/v1/models`,
					},
				},
			},
			{
				Method:      "GET",
				Path:        "/models/{model}",
				Description: "Show detailed information about a specific model.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns model details including metadata, capabilities, and configuration.",
				},
				Examples: []example{
					{
						Description: "Show model details:",
						Code:        `curl -X GET http://localhost:8080/v1/models/qwen3-8b-q8_0`,
					},
				},
			},
			{
				Method:      "GET",
				Path:        "/models/ps",
				Description: "List currently loaded/running models in the cache.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns a list of running models with id, owned_by, model_family, size, expires_at, and active_streams.",
				},
				Examples: []example{
					{
						Description: "List running models:",
						Code:        `curl -X GET http://localhost:8080/v1/models/ps`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/models/index",
				Description: "Rebuild the model index for fast model access.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns empty response on success.",
				},
				Examples: []example{
					{
						Description: "Rebuild model index:",
						Code:        `curl -X POST http://localhost:8080/v1/models/index`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/models/pull",
				Description: "Pull/download a model from a URL. Returns streaming progress updates.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
					{Name: "Content-Type", Description: "Must be application/json", Required: true},
				},
				RequestBody: &requestBody{
					ContentType: "application/json",
					Fields: []field{
						{Name: "model_url", Type: "string", Required: true, Description: "URL to the model GGUF file"},
						{Name: "proj_url", Type: "string", Required: false, Description: "URL to the projection file (for vision/audio models)"},
					},
				},
				Response: &response{
					ContentType: "text/event-stream",
					Description: "Streams download progress as Server-Sent Events.",
				},
				Examples: []example{
					{
						Description: "Pull a model from HuggingFace:",
						Code: `curl -X POST http://localhost:8080/v1/models/pull \
  -H "Content-Type: application/json" \
  -d '{
    "model_url": "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
  }'`,
					},
				},
			},
			{
				Method:      "DELETE",
				Path:        "/models/{model}",
				Description: "Remove a model from the server.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns empty response on success.",
				},
				Examples: []example{
					{
						Description: "Remove a model:",
						Code:        `curl -X DELETE http://localhost:8080/v1/models/qwen3-8b-q8_0`,
					},
				},
			},
		},
	}
}

func catalogEndpoints() endpointGroup {
	return endpointGroup{
		Name:        "Catalog",
		Description: "Browse and pull models from the curated model catalog.",
		Endpoints: []endpoint{
			{
				Method:      "GET",
				Path:        "/catalog",
				Description: "List all models available in the catalog.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns a list of catalog models with id, category, owned_by, model_family, and capabilities.",
				},
				Examples: []example{
					{
						Description: "List catalog models:",
						Code:        `curl -X GET http://localhost:8080/v1/catalog`,
					},
				},
			},
			{
				Method:      "GET",
				Path:        "/catalog/filter/{filter}",
				Description: "List catalog models filtered by category.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns a filtered list of catalog models.",
				},
				Examples: []example{
					{
						Description: "Filter catalog by category:",
						Code:        `curl -X GET http://localhost:8080/v1/catalog/filter/embedding`,
					},
				},
			},
			{
				Method:      "GET",
				Path:        "/catalog/{model}",
				Description: "Show detailed information about a catalog model.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns full catalog model details including files, capabilities, and metadata.",
				},
				Examples: []example{
					{
						Description: "Show catalog model details:",
						Code:        `curl -X GET http://localhost:8080/v1/catalog/qwen3-8b-q8_0`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/catalog/pull/{model}",
				Description: "Pull a model from the catalog by ID. Returns streaming progress updates.",
				Auth:        "Optional when auth is enabled.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for authentication", Required: false},
				},
				Response: &response{
					ContentType: "text/event-stream",
					Description: "Streams download progress as Server-Sent Events.",
				},
				Examples: []example{
					{
						Description: "Pull a catalog model:",
						Code:        `curl -X POST http://localhost:8080/v1/catalog/pull/qwen3-8b-q8_0`,
					},
				},
			},
		},
	}
}

func securityEndpoints() endpointGroup {
	return endpointGroup{
		Name:        "Security",
		Description: "Manage security tokens and private keys for authentication.",
		Endpoints: []endpoint{
			{
				Method:      "POST",
				Path:        "/security/token/create",
				Description: "Create a new security token with specified permissions and duration.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
					{Name: "Content-Type", Description: "Must be application/json", Required: true},
				},
				RequestBody: &requestBody{
					ContentType: "application/json",
					Fields: []field{
						{Name: "admin", Type: "boolean", Required: false, Description: "Whether the token has admin privileges"},
						{Name: "duration", Type: "duration", Required: true, Description: "Token validity duration (e.g., '24h', '720h')"},
						{Name: "endpoints", Type: "object", Required: true, Description: "Map of endpoint names to rate limit configurations"},
					},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns the created token string.",
				},
				Examples: []example{
					{
						Description: "Create a token with chat-completions access:",
						Code: `curl -X POST http://localhost:8080/v1/security/token/create \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "admin": false,
    "duration": "24h",
    "endpoints": {
      "chat-completions": {"limit": 1000, "window": "day"},
      "embeddings": {"limit": 0, "window": ""}
    }
  }'`,
					},
				},
			},
			{
				Method:      "GET",
				Path:        "/security/keys",
				Description: "List all private keys in the system.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns a list of keys with id and created timestamp.",
				},
				Examples: []example{
					{
						Description: "List all keys:",
						Code: `curl -X GET http://localhost:8080/v1/security/keys \
  -H "Authorization: Bearer $KRONK_TOKEN"`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/security/keys/add",
				Description: "Create a new private key and add it to the keystore.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns empty response on success.",
				},
				Examples: []example{
					{
						Description: "Add a new key:",
						Code: `curl -X POST http://localhost:8080/v1/security/keys/add \
  -H "Authorization: Bearer $KRONK_TOKEN"`,
					},
				},
			},
			{
				Method:      "POST",
				Path:        "/security/keys/remove/{keyid}",
				Description: "Remove a private key from the keystore by its ID.",
				Auth:        "Required when auth is enabled. Admin token required.",
				Headers: []header{
					{Name: "Authorization", Description: "Bearer token for admin authentication", Required: true},
				},
				Response: &response{
					ContentType: "application/json",
					Description: "Returns empty response on success.",
				},
				Examples: []example{
					{
						Description: "Remove a key:",
						Code: `curl -X POST http://localhost:8080/v1/security/keys/remove/abc123 \
  -H "Authorization: Bearer $KRONK_TOKEN"`,
					},
				},
			},
		},
	}
}
