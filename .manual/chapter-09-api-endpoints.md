# Chapter 9: API Endpoints

## Table of Contents

- [9.1 API Conventions](#91-api-conventions)
- [9.2 Endpoint Overview](#92-endpoint-overview)
- [9.3 Chat Completions and Tool Calls](#93-chat-completions-and-tool-calls)
- [9.4 Responses API](#94-responses-api)
- [9.5 Anthropic Messages API](#95-anthropic-messages-api)
- [9.6 Embeddings](#96-embeddings)
- [9.7 Reranking](#97-reranking)
- [9.8 Tokenization](#98-tokenization)
- [9.9 Models and Audio Transcription](#99-models-and-audio-transcription)

---

Kronk exposes several familiar inference API formats. This chapter describes
their wire contracts and the Kronk-specific details needed to use them. See
[Chapter 10](https://www.kronkai.com/manual#chapter-10-request-parameters) for generation and sampling
parameters.

## 9.1 API Conventions

The examples use the default server address, `http://localhost:11435`. JSON
endpoints accept `Content-Type: application/json`. Streaming endpoints use
Server-Sent Events (SSE).

When server authentication is enabled, inference requests require a bearer
token with access to the requested endpoint:

```text
Authorization: Bearer <token>
```

Authentication is bypassed only when the server is configured with
authentication disabled. See [Chapter 12](https://www.kronkai.com/manual#chapter-12-security-and-authentication)
for token creation, endpoint grants, and rate limits.

Application errors use a top-level code and message:

```json
{
  "code": "invalid_argument",
  "message": "missing model field"
}
```

The HTTP status reflects the error. Depending on the failure, clients may see
statuses such as 400, 401, 403, 404, 409, 429, 500, 501, or 503.

## 9.2 Endpoint Overview

| Endpoint                       | Method | Purpose                                |
| ------------------------------ | ------ | -------------------------------------- |
| `/v1/chat/completions`         | POST   | OpenAI-style chat completions          |
| `/v1/responses`                | POST   | OpenAI Responses API                   |
| `/v1/messages`                 | POST   | Anthropic Messages API                 |
| `/v1/embeddings`               | POST   | Text embeddings                        |
| `/v1/rerank`                   | POST   | Document reranking                     |
| `/v1/reranking`                | POST   | Alias for `/v1/rerank`                 |
| `/v1/tokenize`                 | POST   | Count tokens for text                  |
| `/v1/models`                   | GET    | List locally available models          |
| `/v1/audio/transcriptions`     | POST   | Transcribe audio with Bucky            |

## 9.3 Chat Completions and Tool Calls

`POST /v1/chat/completions` accepts an OpenAI-style `model` and `messages`
request:

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "messages": [
    {"role": "system", "content": "Be concise."},
    {"role": "user", "content": "What is the capital of France?"}
  ]
}
```

A non-streaming response contains one or more `choices`, an assistant
`message`, a `finish_reason`, and token `usage`. Thinking models can also return
`reasoning_content`. Set the top-level `enable_thinking` boolean to request or
suppress thinking when the model and its chat template support that option.

Use `max_completion_tokens` to set the output-token limit. The legacy
`max_tokens` name remains supported; if both are supplied,
`max_completion_tokens` takes precedence. Kronk does not support custom stop
strings, so requests containing `stop` are rejected rather than silently
ignored. A response that reaches its output-token limit has
`finish_reason: "length"`.

Set `"stream": true` to receive chat completion chunks as SSE records:

```text
data: {"id":"chatcmpl-...","object":"chat.completion.chunk",...}

data: [DONE]
```

By default, the terminal streaming chunk includes `usage`. To omit usage from
all streaming chunks, set `"stream_options": {"include_usage": false}`. This
option affects only the streaming wire response; it does not change generation
or server-side accounting.

Compatible tool-call parsers emit an OpenAI-style activity delta as soon as a
function name is known. The terminal chunk reconciles every completed tool
call, including its arguments, so clients that consume only the final chunk
continue to work unchanged.

`usage.output_tokens` is the sum of reasoning and completion tokens, and
`usage.total_tokens` is prompt plus output tokens. Generated control and
tool-call syntax counts toward output usage even when a parser buffers it
instead of exposing it as assistant text. `usage.reasoning_tokens` and
`usage.completion_tokens` provide the output breakdown.

### Tool calls

Add OpenAI-style function definitions in `tools` and use
`"tool_choice": "auto"` to let the model select one. Tool calling requires a
compatible model, chat template, and output parser; adding `tools` cannot give
an incompatible model tool-calling ability. Set `tool_choice` to the name of a
declared function tool to select that tool. Other values are rejected.

When a tool is selected, the assistant message contains `tool_calls` and uses
an empty string for `content`:

```json
{
  "role": "assistant",
  "content": "",
  "tool_calls": [
    {
      "id": "call_abc123",
      "type": "function",
      "function": {
        "name": "get_weather",
        "arguments": "{\"location\":\"Paris\"}"
      }
    }
  ]
}
```

Execute the function in your application, then append the assistant message
and a `role: "tool"` message containing the result and matching
`tool_call_id`. Send the full conversation in the next request. Tool calls can
also stream incrementally. Kronk accepts `"auto"` or the exact name of a
declared function for `tool_choice`; values such as `"none"`, `"required"`,
and forced-function object forms are rejected.

## 9.4 Responses API

`POST /v1/responses` accepts `input` as a string:

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "input": "Explain quantum computing in simple terms."
}
```

It also accepts an array of input messages for conversations. A non-streaming
response places generated messages or function calls in `output`. Tools use
Responses-style tool definitions. As with Chat Completions, `tool_choice` may
be `"auto"` or the name of a declared function tool.

Use `max_output_tokens` to set the Responses output limit. `max_tokens` remains
available as a compatibility alias, but `max_output_tokens` wins when both are
present. When the limit is reached, the response has `status: "incomplete"`,
`completed_at: null`, and:

```json
"incomplete_details": {"reason": "max_output_tokens"}
```

Output items have the same `incomplete` status. Usage reports all generated
output tokens in `output_tokens`, including reasoning and tool-call syntax;
`output_tokens_details.reasoning_tokens` supplies the reasoning subset, and
`total_tokens` includes both input and output.

With `"stream": true`, each SSE record has a named event and matching JSON
payload. A text response commonly includes:

```text
event: response.created
data: {"type":"response.created",...}

event: response.output_text.delta
data: {"type":"response.output_text.delta","delta":"...",...}

event: response.completed
data: {"type":"response.completed",...}
```

Function calls produce corresponding
`response.function_call_arguments.delta` and `.done` events. A token-limited
stream ends with `response.incomplete` instead of `response.completed` and its
embedded response carries the same incomplete details as a non-streaming
response.

## 9.5 Anthropic Messages API

`POST /v1/messages` provides an Anthropic-style interface. `model` and a
nonzero `max_tokens` are required:

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "max_tokens": 256,
  "system": "Be concise.",
  "messages": [
    {"role": "user", "content": "What is the capital of France?"}
  ]
}
```

`system` and message `content` may be strings or arrays of content blocks. The
API supports text, image, `tool_use`, and `tool_result` blocks, subject to the
selected model's capabilities. Anthropic-style tool definitions use `name`,
`description`, and `input_schema`.

With `"stream": true`, Kronk emits Anthropic-style named events including
`message_start`, `content_block_start`, `content_block_delta`,
`content_block_stop`, `message_delta`, and `message_stop`.

Custom `stop_sequences` are not supported and are rejected. A request that
reaches `max_tokens` returns `stop_reason: "max_tokens"`; natural completion
uses `end_turn`, and a completed tool call uses `tool_use`. Streaming and
non-streaming responses use the same mapping.

## 9.6 Embeddings

`POST /v1/embeddings` accepts one string or an array of strings:

```json
{
  "model": "Qwen/Qwen3-Embedding-0.6B-Q8_0",
  "input": ["First document", "Second document"]
}
```

The response contains `object`, `created`, `model`, a `data` array, and
`usage`. Each data item has an `index` and an `embedding` vector. Use an
embedding model; ordinary text-generation models do not provide useful
embedding behavior.

## 9.7 Reranking

`POST /v1/rerank` and `POST /v1/reranking` are equivalent. Supply a reranker
model, a query, and a nonempty string array:

```json
{
  "model": "gpustack/bge-reranker-v2-m3-Q8_0",
  "query": "What is machine learning?",
  "documents": [
    "Machine learning is a branch of artificial intelligence.",
    "The weather is sunny."
  ],
  "top_n": 1,
  "return_documents": true
}
```

Results are sorted by descending relevance and returned in `data`, not
`results`:

```json
{
  "object": "list",
  "created": 1738857600,
  "model": "gpustack/bge-reranker-v2-m3-Q8_0",
  "data": [
    {"index": 0, "relevance_score": 0.91, "document": "Machine learning is a branch of artificial intelligence."}
  ],
  "usage": {"prompt_tokens": 24, "total_tokens": 24}
}
```

Documents are omitted from results by default. Set `return_documents` to
`true` when the response should include their text. `top_n` defaults to all
documents.

## 9.8 Tokenization

`POST /v1/tokenize` returns a token **count**, not token IDs:

```json
{
  "model": "Qwen/Qwen3-8B-Q8_0",
  "input": "The quick brown fox",
  "apply_template": true,
  "add_generation_prompt": true
}
```

`apply_template` defaults to `false`. When enabled, Kronk wraps the input as a
user message and includes chat-template overhead in the count.
`add_generation_prompt` controls the assistant prefix when the template is
applied and defaults to `true`.

```json
{
  "object": "tokenize",
  "created": 1738857600,
  "model": "Qwen/Qwen3-8B-Q8_0",
  "tokens": 11
}
```

## 9.9 Models and Audio Transcription

`GET /v1/models` returns an OpenAI-style list of models and configured model
extensions available locally. It is not limited to models currently loaded in
memory. Each item includes `id`, `object`, `created`, and `owned_by`.
`owned_by` comes from model metadata when available and otherwise defaults to
`kronk`.

`POST /v1/audio/transcriptions` accepts multipart audio uploads and uses the
Bucky speech-to-text runtime. Its request fields, formats, and administrative
operations are documented in [Chapter 18](https://www.kronkai.com/manual#1861-request-and-response).
