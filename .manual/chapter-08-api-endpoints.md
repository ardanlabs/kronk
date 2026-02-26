# Chapter 8: API Endpoints

## Table of Contents

- [8.1 Endpoint Overview](#81-endpoint-overview)
- [8.2 Chat Completions](#82-chat-completions)
- [8.3 Responses API](#83-responses-api)
- [8.4 Embeddings](#84-embeddings)
- [8.5 Reranking](#85-reranking)
- [8.6 Tokenize](#86-tokenize)
- [8.7 Tool Calling (Function Calling)](#87-tool-calling-function-calling)
- [8.8 Models List](#88-models-list)
- [8.9 Authentication](#89-authentication)
- [8.10 Error Responses](#810-error-responses)

---



Kronk provides an OpenAI-compatible REST API. This chapter documents the
available endpoints and their usage.

### 8.1 Endpoint Overview

| Endpoint               | Method | Description                                |
| ---------------------- | ------ | ------------------------------------------ |
| `/v1/chat/completions` | POST   | Chat completions (streaming/non-streaming) |
| `/v1/responses`        | POST   | OpenAI Responses API format                |
| `/v1/messages`         | POST   | Anthropic API format                       |
| `/v1/embeddings`       | POST   | Generate embeddings                        |
| `/v1/rerank`           | POST   | Rerank documents                           |
| `/v1/tokenize`         | POST   | Tokenize text input                        |
| `/v1/models`           | GET    | List available models                      |

### 8.2 Chat Completions

Generate chat responses using the familiar OpenAI format.

**Endpoint:** `POST /v1/chat/completions`

**Basic Request:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "system", "content": "You are a helpful assistant."},
      {"role": "user", "content": "What is the capital of France?"}
    ]
  }'
```

**Request Parameters:**

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "system", "content": "System prompt" },
    { "role": "user", "content": "User message" },
    { "role": "assistant", "content": "Previous response" },
    { "role": "user", "content": "Follow-up question" }
  ],
  "temperature": 0.8,
  "top_p": 0.9,
  "top_k": 40,
  "max_tokens": 2048,
  "stream": true
}
```

**Streaming Response:**

With `"stream": true`, responses are sent as Server-Sent Events:

```
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk",...}

data: [DONE]
```

**Non-Streaming Response:**

```json
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "Qwen3-8B-Q8_0",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "The capital of France is Paris."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 25,
    "completion_tokens": 8,
    "total_tokens": 33
  }
}
```

**Reasoning Models:**

For models with thinking/reasoning support (like Qwen3):

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [...],
  "enable_thinking": true
}
```

The response includes `reasoning_content` in the message.

To disable thinking:

```json
{
  "enable_thinking": false
}
```

### 8.3 Responses API

OpenAI's newer Responses API format, used by some clients.

**Endpoint:** `POST /v1/responses`

**Request:**

```shell
curl http://localhost:8080/v1/responses \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "Explain quantum computing in simple terms."
  }'
```

The `input` field can be a string or an array of message objects.

**Streaming Events:**

The Responses API uses a different event format:

```
event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"The",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":" answer",...}

event: response.completed
data: {"type":"response.completed",...}
```

### 8.4 Embeddings

Generate vector embeddings for text.

**Endpoint:** `POST /v1/embeddings`

**Request:**

```shell
curl http://localhost:8080/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "embeddinggemma-300m-qat-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog."
  }'
```

**Multiple Inputs:**

```json
{
  "model": "embeddinggemma-300m-qat-Q8_0",
  "input": [
    "First document to embed.",
    "Second document to embed.",
    "Third document to embed."
  ]
}
```

**Response:**

```json
{
  "object": "list",
  "data": [
    {
      "object": "embedding",
      "index": 0,
      "embedding": [0.123, -0.456, 0.789, ...]
    }
  ],
  "model": "embeddinggemma-300m-qat-Q8_0",
  "usage": {
    "prompt_tokens": 10,
    "total_tokens": 10
  }
}
```

### 8.5 Reranking

Score and reorder documents by relevance to a query.

**Endpoint:** `POST /v1/rerank`

**Request:**

```shell
curl http://localhost:8080/v1/rerank \
  -H "Content-Type: application/json" \
  -d '{
    "model": "bge-reranker-v2-m3-Q8_0",
    "query": "What is machine learning?",
    "documents": [
      "Machine learning is a subset of artificial intelligence.",
      "The weather today is sunny.",
      "Deep learning uses neural networks.",
      "I like pizza."
    ],
    "top_n": 2
  }'
```

**Response:**

```json
{
  "object": "list",
  "results": [
    {
      "index": 0,
      "relevance_score": 0.95,
      "document": "Machine learning is a subset of artificial intelligence."
    },
    {
      "index": 2,
      "relevance_score": 0.82,
      "document": "Deep learning uses neural networks."
    }
  ],
  "model": "bge-reranker-v2-m3-Q8_0",
  "usage": {
    "prompt_tokens": 45,
    "total_tokens": 45
  }
}
```

### 8.6 Tokenize

Get the token count for a text input. Works with any model type.

**Endpoint:** `POST /v1/tokenize`

**Parameters:**

| Field                   | Type      | Required | Description                                                                                                                                                  |
| ----------------------- | --------- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `model`                 | `string`  | Yes      | Model ID (e.g., `Qwen3-8B-Q8_0`). Works with any model type.                                                                                                 |
| `input`                 | `string`  | Yes      | The text to tokenize.                                                                                                                                        |
| `apply_template`        | `boolean` | No       | If true, wraps the input as a user message and applies the model's chat template before tokenizing. The count includes template overhead. Defaults to false. |
| `add_generation_prompt` | `boolean` | No       | When `apply_template` is true, controls whether the assistant role prefix is appended to the prompt. Defaults to true.                                       |

**Request (raw text):**

```shell
curl http://localhost:8080/v1/tokenize \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog"
  }'
```

**Request (with template):**

```shell
curl http://localhost:8080/v1/tokenize \
  -H "Authorization: Bearer $KRONK_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "input": "The quick brown fox jumps over the lazy dog",
    "apply_template": true
  }'
```

**Response:**

```json
{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen3-8B-Q8_0",
  "tokens": 11
}
```

When `apply_template` is true, the token count will be higher than raw text
because it includes template overhead (role markers, separators, and the
generation prompt).

### 8.7 Tool Calling (Function Calling)

Kronk supports OpenAI-compatible tool calling, allowing models to request
function executions that you handle in your application.

**Request with Tools:**

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "Qwen3-8B-Q8_0",
    "messages": [
      {"role": "user", "content": "What is the weather in Paris?"}
    ],
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "Get current weather for a location",
          "parameters": {
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
      }
    ],
    "tool_choice": "auto"
  }'
```

**Tool Choice Options:**

- `"auto"` - Model decides whether to call tools (default)
- `"none"` - Never call tools
- `{"type": "function", "function": {"name": "get_weather"}}` - Force specific tool

**Response with Tool Calls:**

```json
{
  "id": "chatcmpl-xxx",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_abc123",
            "type": "function",
            "function": {
              "name": "get_weather",
              "arguments": "{\"location\": \"Paris\"}"
            }
          }
        ]
      },
      "finish_reason": "tool_calls"
    }
  ]
}
```

**Handling Tool Results:**

After executing the tool, send the result back:

```json
{
  "model": "Qwen3-8B-Q8_0",
  "messages": [
    { "role": "user", "content": "What is the weather in Paris?" },
    {
      "role": "assistant",
      "content": null,
      "tool_calls": [
        {
          "id": "call_abc123",
          "type": "function",
          "function": {
            "name": "get_weather",
            "arguments": "{\"location\": \"Paris\"}"
          }
        }
      ]
    },
    {
      "role": "tool",
      "tool_call_id": "call_abc123",
      "content": "{\"temperature\": 18, \"condition\": \"sunny\"}"
    }
  ]
}
```

**Streaming with Tool Calls:**

Tool call arguments stream incrementally:

```
data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":" \"Paris\"}"}}]}}]}
```

### 8.8 Models List

Get available models.

**Endpoint:** `GET /v1/models`

**Request:**

```shell
curl http://localhost:8080/v1/models
```

**Response:**

```json
{
  "object": "list",
  "data": [
    {
      "id": "Qwen3-8B-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    },
    {
      "id": "embeddinggemma-300m-qat-Q8_0",
      "object": "model",
      "owned_by": "kronk"
    }
  ]
}
```

### 8.9 Authentication

When authentication is enabled, include the token in requests:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-token-here" \
  -d '{...}'
```

See [Chapter 11: Security & Authentication](#chapter-11-security--authentication)
for details on token management.

### 8.10 Error Responses

Errors follow a standard format:

```json
{
  "error": {
    "code": "invalid_argument",
    "message": "missing model field"
  }
}
```

**Common Error Codes:**

- `invalid_argument` - Missing or invalid request parameters
- `not_found` - Model not found
- `internal` - Server error during processing
- `unauthenticated` - Missing or invalid authentication token

---
