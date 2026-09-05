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
- [9.10 Kronk Administration](#910-kronk-administration)
- [9.11 Bucky Administration](#911-bucky-administration)
- [9.12 Operations and Evaluation](#912-operations-and-evaluation)
- [9.13 Security Administration](#913-security-administration)

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
| `/v1/models/{model}`           | GET    | Retrieve one locally available model   |
| `/v1/audio/transcriptions`     | POST   | Transcribe audio with Bucky            |

Sections 9.10 through 9.13 inventory the administration, diagnostics, and
evaluation endpoints used by the CLI and BUI. Administration endpoints are
open when administration authentication is disabled. When it is enabled, they
require an administrator token. `GET /v1/models` and
`GET /v1/models/{model}` instead follow inference authentication and do not
require a separate endpoint grant.

## 9.3 Chat Completions and Tool Calls

`POST /v1/chat/completions` accepts an OpenAI-style `model` and `messages`
request:

```json
{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
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

Kronk currently generates one choice per request. The optional `n` field may be
omitted, `null`, or `1`; other values are rejected because multiple choices are
not supported.

Use `max_completion_tokens` to set the output-token limit. The legacy
`max_tokens` name remains supported; if both are supplied,
`max_completion_tokens` takes precedence. Use `stop` with a string or an array
of up to four strings to end generation when Kronk encounters one of those
sequences. The matched sequence is omitted from the response. A custom stop
has `finish_reason: "stop"`; a response that reaches its output-token limit has
`finish_reason: "length"`.

Set `"stream": true` to receive chat completion chunks as SSE records:

```text
data: {"id":"chatcmpl-...","object":"chat.completion.chunk",...}

data: [DONE]
```

By default, streaming chunks omit `usage`. To request usage, set
`"stream_options": {"include_usage": true}`. Each completion chunk then has
`"usage": null`, and Kronk sends one additional chunk before `[DONE]` with an
empty `choices` array and the final usage totals. This option affects the
streaming response; it does not change generation or server-side accounting.

Compatible tool-call parsers emit an OpenAI-style activity delta as soon as a
function name is known. Once parsing completes, Kronk emits the completed
arguments in a nonterminal tool-call delta followed by an empty terminal delta
with `finish_reason: "tool_calls"`.

`usage.completion_tokens` includes all generated tokens, including reasoning,
control, and tool-call syntax that a parser may buffer instead of exposing as
assistant text. `usage.completion_tokens_details.reasoning_tokens` reports the
reasoning subset. `usage.total_tokens` is the sum of `prompt_tokens` and
`completion_tokens`.

### Tool calls

Add OpenAI-style function definitions in `tools` and use
`"tool_choice": "auto"` to let the model select one. Tool calling requires a
compatible model, chat template, and output parser; adding `tools` cannot give
an incompatible model tool-calling ability. Use `"none"` to withhold tools and
prevent structured tool-call output, `"required"` to request a call from a
compatible model template, or the OpenAI forced-function object to limit the
request to one declared function. Required and forced selection remain subject
to the model and template honoring the requested mode.

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
also stream incrementally. Chat Completions selects a specific function with
`{"type":"function","function":{"name":"get_weather"}}`.

## 9.4 Responses API

`POST /v1/responses` accepts `input` as a string:

```json
{
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "input": "Explain quantum computing in simple terms."
}
```

It also accepts an array of input messages for conversations. A non-streaming
response places generated messages or function calls in `output`. Tools use
Responses-style tool definitions. `tool_choice` accepts `"none"`, `"auto"`,
or `"required"`. Select a specific function with the Responses form
`{"type":"function","name":"get_weather"}`.

Use `max_output_tokens` to set the Responses output limit. `max_tokens` remains
available as a compatibility alias, but `max_output_tokens` wins when both are
present. The Responses API does not support `stop`, so requests containing it
are rejected. When the output-token limit is reached, the response has
`status: "incomplete"`, `completed_at: null`, and:

```json
"incomplete_details": {"reason": "max_output_tokens"}
```

Output items have the same `incomplete` status. Usage reports all generated
output tokens in `output_tokens`, including reasoning and tool-call syntax;
`output_tokens_details.reasoning_tokens` supplies the reasoning subset, and
`total_tokens` includes both input and output.

With `"stream": true`, each SSE record has a named event and matching JSON
payload. Streaming usage is omitted unless the request sets
`"stream_options": {"include_usage": true}`. A text response commonly
includes:

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
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
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
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
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
  "model": "unsloth/Qwen3-1.7B-UD-Q8_K_XL",
  "tokens": 11
}
```

## 9.9 Models and Audio Transcription

`GET /v1/models` returns an OpenAI-style list of models and configured model
extensions available locally. It is not limited to models currently loaded in
memory. Each item includes `id`, `object`, `created`, and `owned_by`. The `id`
uses the canonical `provider/modelID` form.
`owned_by` comes from model metadata when available and otherwise defaults to
`kronk`.

`GET /v1/models/{model}` returns the corresponding OpenAI-style model object
for one model ID. It returns `404 Not Found` when the model is not available.

`POST /v1/audio/transcriptions` accepts multipart audio uploads and uses the
Bucky speech-to-text runtime. Its request fields, formats, and administrative
operations are documented in [Chapter 18](https://www.kronkai.com/manual#1861-request-and-response).

## 9.10 Kronk Administration

These routes manage the llama.cpp runtime, local GGUF models, and the personal
model catalog. Mutating routes may stream progress or perform network and disk
operations. Clients should use the exact `/v1/kronk/...` prefix; the shorter
`/v1/libs`, `/v1/models/pull`, and `/v1/catalog` forms are not aliases.

### Libraries

| Method and path | Purpose |
| ---------------- | ------- |
| `GET /v1/kronk/libs` | Show the active llama.cpp library installation and upgrade state |
| `GET /v1/kronk/libs/integrity` | Hash and verify the selected llama.cpp bundle against Yzma's release manifest |
| `GET /v1/kronk/libs/combinations` | List supported operating-system, architecture, and processor combinations |
| `GET /v1/kronk/libs/installs` | List installed library bundles |
| `POST /v1/kronk/libs/pull` | Install a library bundle and stream progress |
| `DELETE /v1/kronk/libs/installs` | Remove the bundle selected by `arch`, `os`, and `processor` query parameters |

`POST /v1/kronk/libs/pull` accepts optional `arch`, `os`, `processor`, and
`version` query parameters. Supply `arch`, `os`, and `processor` together for a
cross-platform bundle; omit all three to operate on the active platform. A
version can be externally pinned as `VERSION@sha256:<64-hex-digest>`. Yzma
checks the raw release-manifest bytes against that digest and then verifies the
selected archive before extraction.

`GET /v1/kronk/libs/integrity` hashes the installed files and compares them
with the file digests in Yzma's release manifest. Its optional `version` query
parameter accepts the same pinned syntax, allowing the caller to supply the
trusted manifest digest rather than trusting the server's install record or
manifest host. The response identifies the backend, version, platform triple,
overall result, per-file states, and changed, missing, or unexpected counts.
Kronk's default llama.cpp version already carries its published manifest
digest, so default downloads and startup verification use the authenticated
manifest without an operator-supplied version.

Verification requires a Yzma install record and a release manifest containing
per-file hashes. A release or upstream asset with archive hashes only returns
an error rather than claiming the installed files are verified.

### Models

| Method and path | Purpose |
| ---------------- | ------- |
| `GET /v1/kronk/models` | List locally installed GGUF models with Kronk metadata |
| `GET /v1/kronk/models/` | Return the missing-model validation error for an empty model ID |
| `GET /v1/kronk/models/integrity` | List local artifact digests and persisted verification evidence without hashing model files |
| `GET /v1/kronk/models/integrity/{model}` | Return integrity information for one local model without inspecting other models |
| `GET /v1/kronk/models/{model}` | Show detailed metadata and effective configuration for one model |
| `GET /v1/kronk/models/ps` | List models currently loaded in the pool |
| `GET /v1/kronk/models/imc-sessions` | List active incremental-message-cache sessions |
| `POST /v1/kronk/models/index` | Rebuild the local model index |
| `POST /v1/kronk/models/pull` | Download a model and optional companion files and stream progress |
| `POST /v1/kronk/models/autotune` | Resolve an automatically tuned runtime configuration for a model |
| `POST /v1/kronk/models/vram` | Estimate model memory use for the requested runtime settings |
| `POST /v1/kronk/models/unload` | Unload a model or playground instance from the pool |
| `DELETE /v1/kronk/models/{model}` | Remove a locally installed model |

The native model list and detail routes are distinct from the OpenAI-compatible
`GET /v1/models` and `GET /v1/models/{model}` routes. Use the native routes for
administration metadata and effective Kronk configuration.

`GET /v1/kronk/models/integrity` returns each indexed physical model with its
weight shards, projection, and MTP artifacts. Each artifact includes the local
filename and size, its Hugging Face SHA-256 digest when available, and one of
these evidence states. For example:

```json
{
  "object": "model_integrity.list",
  "data": [
    {
      "id": "Qwen3-VL-30B-Q4_K_M",
      "owned_by": "unsloth",
      "model_family": "Qwen3-VL-30B-GGUF",
      "status": "unavailable",
      "verified": false,
      "artifacts": [
        {
          "role": "weights",
          "filename": "Qwen3-VL-30B-Q4_K_M.gguf",
          "digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
          "size": 18456789012,
          "status": "verified",
          "verified": true,
          "verified_at": "2026-08-07T14:35:09Z"
        },
        {
          "role": "projection",
          "filename": "mmproj-Qwen3-VL-30B-Q4_K_M.gguf",
          "size": 734567890,
          "status": "unavailable",
          "verified": false,
          "reason": "digest_unavailable"
        }
      ]
    }
  ]
}
```

Use `GET /v1/kronk/models/integrity/{model}` when only one model is needed. It
reads the index and inspects sidecars and filesystem metadata only for that
model. The response is the matching model object directly, without the list
envelope:

```json
{
  "id": "Qwen3-VL-30B-Q4_K_M",
  "owned_by": "unsloth",
  "model_family": "Qwen3-VL-30B-GGUF",
  "status": "verified",
  "verified": true,
  "artifacts": [
    {
      "role": "weights",
      "filename": "Qwen3-VL-30B-Q4_K_M.gguf",
      "digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
      "size": 18456789012,
      "status": "verified",
      "verified": true,
      "verified_at": "2026-08-07T14:35:09Z"
    }
  ]
}
```

The targeted route returns `404 Not Found` when the model is not present in the
local index. It is preferable to the collection route on installations with a
large model inventory when a caller needs only one model.

The response envelope and model fields are:

| Field | Meaning |
| ----- | ------- |
| `object` | Always `model_integrity.list`. |
| `data` | Models physically present in the local model index. Configured extension aliases are not duplicated here. |
| `id` | Kronk model ID from the local index. |
| `owned_by` | Provider or organization directory containing the model. |
| `model_family` | Model-family directory containing the artifacts. |
| `status` | Least trustworthy status among the model's required artifacts: `unavailable`, then `stale`, then `unverified`, then `verified`. |
| `verified` | `true` only when every required artifact has status `verified`. |
| `artifacts` | Physical weight shards followed by an optional projection and optional MTP drafter. |

Each artifact provides:

| Field | Meaning |
| ----- | ------- |
| `role` | `weights`, `projection`, or `mtp`. Each weight shard is a separate artifact. |
| `filename` | Basename of the local artifact. |
| `digest` | Hugging Face Git LFS SHA-256 identity in `sha256:<hex>` form. Omitted when unavailable. |
| `size` | Current local file size in bytes. It is `0` when an indexed file is absent. |
| `status` | Current evidence state described below. |
| `verified` | Convenience boolean that is `true` only for status `verified`. |
| `verified_at` | UTC time of the last successful full-file verification. Omitted when no usable verification record exists. |
| `reason` | Machine-readable explanation when applicable: `digest_unavailable` or `file_metadata_changed`. |

Artifact statuses mean:

- `verified` — the published digest, persisted verification record, and current
  file size and modification time agree;
- `unverified` — a published digest exists, but no persisted verification
  record exists;
- `stale` — persisted verification or current file metadata no longer agrees;
- `unavailable` — the published digest sidecar is missing or malformed.

This endpoint reads only the model index, small sidecar files, and filesystem
metadata. It does not hash model contents or freshly verify their bytes. A
`verified` result means Kronk previously hashed that artifact successfully and
the tracked file metadata has not changed since. It does not prove the current
bytes through a new content read.

### Catalog

| Method and path | Purpose |
| ---------------- | ------- |
| `GET /v1/kronk/catalog` | List personal catalog entries and local validation state |
| `GET /v1/kronk/catalog/{id...}` | Show one catalog entry; the ID may contain slashes |
| `POST /v1/kronk/catalog/lookup` | List GGUF files for a HuggingFace repository or URL |
| `POST /v1/kronk/catalog/resolve` | Resolve a model source to canonical download files without downloading |
| `POST /v1/kronk/catalog/reconcile` | Reconcile catalog metadata with locally installed models |
| `DELETE /v1/kronk/catalog/{id...}` | Remove a catalog entry and its downloaded and cached files |

`POST /v1/kronk/catalog/lookup` accepts `{"input":"..."}`. The resolve route
accepts `{"source":"..."}` and may add successfully resolved metadata to the
personal catalog even though it does not download model files.

## 9.11 Bucky Administration

The Bucky management API mirrors the library and model lifecycle for the
whisper.cpp backend:

| Method and path | Purpose |
| ---------------- | ------- |
| `GET /v1/bucky/libs` | Show the active whisper.cpp library installation |
| `GET /v1/bucky/libs/integrity` | Hash and verify the selected whisper.cpp bundle against Bucky's release manifest |
| `GET /v1/bucky/libs/combinations` | List supported platform combinations |
| `GET /v1/bucky/libs/installs` | List installed library bundles |
| `POST /v1/bucky/libs/pull` | Install a library bundle and stream progress |
| `DELETE /v1/bucky/libs/installs` | Remove the selected library bundle |
| `GET /v1/bucky/models` | List installed whisper models |
| `GET /v1/bucky/models/catalog` | List the bundled short-name model catalog |
| `POST /v1/bucky/models/pull` | Download a whisper model and stream progress |
| `GET /v1/bucky/models/{model}/details` | Show model header and file details |
| `DELETE /v1/bucky/models/{model}` | Remove an installed whisper model |

`POST /v1/bucky/libs/pull` accepts the same optional platform and `version`
query parameters as the Kronk library route. Bucky always verifies the selected
archive against its release manifest before extraction. A version in
`VERSION@sha256:<64-hex-digest>` form also authenticates the manifest itself.
The default Bucky version already includes its published manifest digest.

`GET /v1/bucky/libs/integrity` hashes the installed files. With no `version`
query parameter, Kronk uses the version recorded for the active installation;
the default installation therefore retains its authenticated manifest pin. The
response includes `manifest_authenticated` and `source` in addition to the
common library integrity fields.

See [Chapter 18](https://www.kronkai.com/manual#chapter-18-bucky-audio-transcription)
for installation, model naming, transcription formats, and Bucky-specific
runtime behavior.

## 9.12 Operations and Evaluation

| Method and path | Purpose |
| ---------------- | ------- |
| `GET /v1/pool/budget` | Report shared host and device memory budgets and current reservations |
| `GET /v1/devices` | List detected compute devices |
| `GET /v1/diagnose` | Return the JSON diagnostic report; use `bench=true` to include a benchmark |
| `GET /v1/accuracy/functions` | List functions available to the code-recall accuracy test |
| `POST /v1/accuracy/test` | Run one code-recall comparison using `model` and `function` |
| `POST /v1/efficiency/run` | Warm a model, run one prompt, and report throughput measurements |

`GET /v1/diagnose` also accepts optional `model` and `processor` query
parameters for its benchmark. The accuracy request body is
`{"model":"...","function":"..."}`. The efficiency request body contains
`model`, `prompt`, and an optional positive `max_tokens`, which defaults to
512. These evaluation routes can load models and may take several minutes.

## 9.13 Security Administration

| Method and path | Purpose |
| ---------------- | ------- |
| `POST /v1/security/token/create` | Create a token with administrator status or endpoint grants and quotas |
| `GET /v1/security/keys` | List signing keys |
| `POST /v1/security/keys/add` | Create a signing key |
| `POST /v1/security/keys/remove/{keyid}` | Remove a non-master signing key and revoke its tokens |

These routes are authorized by the authentication service. In protected modes
they require an administrator token. Token creation accepts `admin`, `duration`,
and an `endpoints` map of endpoint grants and rate limits. See
[Chapter 12](https://www.kronkai.com/manual#chapter-12-security-and-authentication)
for the request model, key rotation, and the effects of deleting a signing key.
