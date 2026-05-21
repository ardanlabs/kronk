# ==============================================================================
# Kronk Endpoints

curl-liveness:
	curl -i -X GET http://localhost:11435/v1/liveness

curl-readiness:
	curl -i -X GET http://localhost:11435/v1/readiness

curl-kronk-chat:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "gpt-oss-20b-Q8_0", \
		"stream": true, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "Hello model" \
			} \
		] \
    }'

curl-kronk-chat-load:
	for i in {1..3}; do \
		curl -i -X POST http://localhost:11435/v1/chat/completions \
		-H "Authorization: Bearer ${KRONK_TOKEN}" \
		-H "Content-Type: application/json" \
		-d '{ \
			"model": "gpt-oss-20b-Q8_0", \
			"stream": true, \
			"messages": [ \
				{ \
					"role": "user", \
					"content": "Hello model" \
				} \
			] \
		}' & \
	done; wait

FILE_GIRAFFE := $(shell base64 < examples/samples/giraffe.jpg)

curl-kronk-chat-image:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "What is in this image?" \
			}, \
			{ \
				"role": "user", \
				"content": "$(FILE_GIRAFFE)" \
			} \
		] \
    }'

curl-kronk-chat-openai-image:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": [ \
					{"type": "text", "text": "What is in this image?"}, \
					{"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,'$(FILE_GIRAFFE)'"}} \
				] \
			} \
		] \
    }'

curl-kronk-chat-gpt:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "gpt-oss-20b-Q8_0", \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "Hello model" \
			} \
		] \
    }'

curl-kronk-chat-tool:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"stream": true, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "what is the weather in NYC" \
			} \
		], \
		"tool_selection": "auto", \
		"tools": [ \
			{ \
				"type": "function", \
				"function": { \
					"name": "get_weather", \
					"description": "Get the current weather for a location", \
					"parameters": { \
						"type": "object", \
						"properties": { \
							"location": { \
								"type": "string", \
								"description": "The location to get the weather for, e.g. San Francisco, CA" \
							} \
						}, \
						"required": ["location"] \
					} \
				} \
			} \
		] \
    }'

curl-kronk-embeddings:
	curl -i -X POST http://localhost:11435/v1/embeddings \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "embeddinggemma-300m-qat-Q8_0", \
  		"input": "Why is the sky blue?" \
    }'

curl-kronk-rerank:
	curl -i -X POST http://localhost:11435/v1/rerank \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "bge-reranker-v2-m3-Q8_0", \
  		"query": "What is the capital of France?", \
		"documents": [ \
			"Paris is the capital and largest city of France.", \
			"Berlin is the capital of Germany.", \
			"The Eiffel Tower is located in Paris.", \
			"London is the capital of England.", \
			"France is a country in Western Europe." \
		], \
		"top_n": 3, \
		"return_documents": true \
    }'

curl-kronk-responses:
	curl -i -X POST http://localhost:11435/v1/responses \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "cerebras_qwen3-coder-reap-25b-a3b-q8_0", \
		"input": "Hello model" \
    }'

curl-kronk-responses-image:
	curl -i -X POST http://localhost:11435/v1/responses \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"stream": true, \
	 	"model": "Qwen2.5-VL-3B-Instruct-Q8_0", \
		"input": [ \
			{ \
				"type": "input_text", \
				"text": "What is in this image?" \
			}, \
			{ \
				"type": "input_image", \
				"image_url": "data:image/jpeg;base64,'$(FILE_GIRAFFE)'" \
			} \
		] \
    }'

curl-kronk-tool-response:
	curl -i -X POST http://localhost:11435/v1/chat/completions \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
		"model": "Qwen3-8B-Q8_0", \
		"max_tokens": 32768, \
		"temperature": 0.1, \
		"top_p": 0.1, \
		"top_k": 50, \
		"messages": [ \
			{ \
				"role": "user", \
				"content": "What is the weather like in San Fran?" \
			}, \
			{ \
				"role": "assistant", \
				"tool_calls": [ \
					{ \
						"id": "76803ff7-339e-44c4-b51e-769c2b5fa68e", \
						"type": "function", \
						"function": { \
							"name": "tool_get_weather", \
							"arguments": "{\"location\":\"San Francisco\"}" \
						} \
					} \
				] \
			}, \
			{ \
				"role": "tool", \
				"tool_call_id": "76803ff7-339e-44c4-b51e-769c2b5fa68e", \
				"content": "{\"status\":\"SUCCESS\",\"data\":{\"description\":\"The weather in San Francisco, CA is hot and humid\\n\",\"humidity\":80,\"temperature\":28,\"wind_speed\":10}}" \
			} \
		], \
		"tool_selection": "auto", \
		"tools": [ \
			{ \
				"type": "function", \
				"function": { \
					"name": "tool_get_weather", \
					"description": "Get the current weather for a location", \
					"parameters": { \
						"type": "object", \
						"properties": { \
							"location": { \
								"type": "string", \
								"description": "The location to get the weather for, e.g. San Francisco, CA" \
							} \
						}, \
						"required": ["location"] \
					} \
				} \
			} \
		] \
	}'

curl-tokenize:
	curl -i -X POST http://localhost:11435/v1/tokenize \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"input": "The quick brown fox jumps over the lazy dog" \
    }'

curl-tokenize-template:
	curl -i -X POST http://localhost:11435/v1/tokenize \
	 -H "Authorization: Bearer ${KRONK_TOKEN}" \
     -H "Content-Type: application/json" \
     -d '{ \
	 	"model": "Qwen3-8B-Q8_0", \
		"input": "The quick brown fox jumps over the lazy dog", \
		"apply_template": true \
    }'

# ==============================================================================
# MCP Service

# Start the standalone MCP server.
mcp-server:
	go run cmd/server/api/services/mcp/main.go

# Initialize the MCP session. The response includes the Mcp-Session-Id header
# needed for subsequent requests.
# Usage: make curl-mcp-init
curl-mcp-init:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 1, \
		"method": "initialize", \
		"params": { \
			"protocolVersion": "2025-03-26", \
			"capabilities": {}, \
			"clientInfo": {"name": "curl-client", "version": "1.0.0"} \
		} \
	}'

# Send the initialized notification.
# make curl-mcp-initialized SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-initialized:
	curl -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"method": "notifications/initialized" \
	}'

# List available tools.
# make curl-mcp-tools-list SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-tools-list:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 2, \
		"method": "tools/list", \
		"params": {} \
	}'

# Call the web_search tool.
# make curl-mcp-web-search SESSIONID=<Mcp-Session-Id-from-init>
curl-mcp-web-search:
	curl -i -X POST "http://localhost:9000/mcp" \
	-H "Content-Type: application/json" \
	-H "Accept: application/json, text/event-stream" \
	-H "Mcp-Session-Id: $(SESSIONID)" \
	-d '{ \
		"jsonrpc": "2.0", \
		"id": 3, \
		"method": "tools/call", \
		"params": { \
			"name": "web_search", \
			"arguments": {"query": "what is the Model Context Protocol", "count": 5} \
		} \
	}'
