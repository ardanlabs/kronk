# Release Notes

## Index

- [BREAKING CHANGES](#breaking-changes)
  - [Auth Changes](#auth-changes)
  - [Tool Calling Changes](#tool-calling-changes)
  - [Usage Changes](#usage-changes)
  - [Network Binding Changes](#network-binding-changes)
  - [HTTP Error Response Changes](#http-error-response-changes)
  - [Session Storage Changes](#session-storage-changes)
  - [Go SDK Changes](#go-sdk-changes)

# BREAKING CHANGES

## Auth Changes

Kronk now uses one setting to describe API access:

```shell
KRONK_AUTHORIZATION_MODE=<mode>
```

### Choose the intended mode

| Mode             | `/v1/models` discovery | Inference                              | Management APIs |
| ---------------- | ---------------------- | -------------------------------------- | --------------- |
| `open`           | Public                 | Public                                 | Public          |
| `management`     | Public                 | Public                                 | Admin JWT       |
| `authenticated`  | Valid JWT              | Valid JWT; endpoint grant not required | Admin JWT       |
| `full-protected` | Valid JWT              | JWT with matching endpoint grant       | Admin JWT       |

Health endpoints remain public in every mode.

Management APIs include native Kronk model inventory and integrity routes:

```text
GET /v1/kronk/models
GET /v1/kronk/models/integrity
GET /v1/kronk/models/integrity/{model}
```

They also include catalog, device, diagnostic, pool, playground, security, model-management, and library-management routes.

### Recommended mapping from the old settings

| Previous embedded-auth configuration                               | New mode         |
| ------------------------------------------------------------------ | ---------------- |
| `KRONK_AUTH_LOCAL_ENABLED=false`, `KRONK_AUTH_ADMIN_ENABLED=false` | `open`           |
| `KRONK_AUTH_LOCAL_ENABLED=false`, `KRONK_AUTH_ADMIN_ENABLED=true`  | `management`     |
| `KRONK_AUTH_LOCAL_ENABLED=true`                                    | `full-protected` |

`authenticated` is new. Use it when every API client should present a JWT, but inference tokens should not require individual endpoint grants.

For production installations already using endpoint-specific token grants and rate limits, use:

```shell
KRONK_AUTHORIZATION_MODE=full-protected
```

### Remove the legacy settings

Once the deployed Kronk version supports authorization modes, remove these from Kronk’s configuration:

```shell
KRONK_AUTH_LOCAL_ENABLED
KRONK_AUTH_ADMIN_ENABLED
```

An explicit `KRONK_AUTHORIZATION_MODE` overrides them, so leaving them temporarily will not change behavior. Removing them is still recommended to avoid configuration ambiguity.

Keep these settings as appropriate:

```shell
KRONK_WEB_ADMIN_ENABLED
KRONK_AUTH_HOST
KRONK_AUTH_LOCAL_ISSUER
KRONK_AUTH_TLS_ENABLED
KRONK_AUTH_TLS_CA_FILE
KRONK_AUTH_TLS_SERVER_NAME
```

`KRONK_WEB_ADMIN_ENABLED` controls whether the BUI is served. It does not select API permissions.

### Embedded authentication

When `KRONK_AUTH_HOST` is empty, Kronk runs the embedded authentication service. The selected authorization mode automatically configures it; do not set `KRONK_AUTH_LOCAL_ENABLED`.

Example:

```shell
KRONK_AUTHORIZATION_MODE=full-protected
KRONK_WEB_ADMIN_ENABLED=true
```

For protected BUI login, retain the configured admin password digest:

```shell
KRONK_WEB_ADMIN_PASSWORD_SHA256=<sha256>
```

### External authentication

When using a standalone auth service:

```shell
KRONK_AUTHORIZATION_MODE=full-protected
KRONK_AUTH_HOST=auth.example.internal:6000
```

The standalone auth service must actually enforce authentication:

```shell
AUTH_AUTH_ENABLED=true
```

This is important. `KRONK_AUTHORIZATION_MODE` tells the Kronk server which routes must call authentication, but the external auth service determines whether those authentication requests are enforced.

Keep the standalone service’s issuer and existing key storage stable so previously issued JWTs remain valid:

```shell
AUTH_AUTH_ISSUER="kronk project"
```

Enable TLS when the auth service crosses an untrusted network.

#### External auth and the BUI

Protected BUI password login currently uses Kronk’s embedded security store. Therefore, when using an external auth host with `management`, `authenticated`, or `full-protected`, disable the BUI:

```shell
KRONK_WEB_ADMIN_ENABLED=false
```

API clients can continue using bearer JWTs issued by the external auth service.

### Suggested production configuration

For scoped inference tokens:

```shell
KRONK_AUTHORIZATION_MODE=full-protected
```

This provides:

- Public liveness and readiness checks.
- JWT-protected model discovery.
- Endpoint-grant enforcement for inference.
- Admin-only native model, integrity, catalog, tool, and security APIs.

For clients that cannot manage endpoint grants but can send a JWT:

```shell
KRONK_AUTHORIZATION_MODE=authenticated
```

### Post-deployment checks

Assuming:

```shell
export KRONK_URL=http://localhost:11435
export USER_TOKEN=<non-admin-jwt>
export ADMIN_TOKEN=<admin-jwt>
```

Health should remain public:

```shell
curl -i "$KRONK_URL/v1/liveness"
```

Under `authenticated` or `full-protected`, model discovery without a token should fail:

```shell
curl -i "$KRONK_URL/v1/models"
```

Model discovery with any valid JWT should work:

```shell
curl -i \
  -H "Authorization: Bearer $USER_TOKEN" \
  "$KRONK_URL/v1/models"
```

Management should reject a non-admin JWT:

```shell
curl -i \
  -H "Authorization: Bearer $USER_TOKEN" \
  "$KRONK_URL/v1/kronk/models/integrity"
```

Management should accept an admin JWT:

```shell
curl -i \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$KRONK_URL/v1/kronk/models/integrity"
```

Under `full-protected`, inference should accept the user token only when it contains the matching endpoint grant, such as `chat-completions`, `responses`, `messages`, `embeddings`, `rerank`, `tokenize`, or `transcriptions`.

## Tool Calling Changes

Tool selection is now validated and normalized according to the OpenAI Chat
Completions and Responses API shapes. Requests that previously relied on a bare
function name or an unsupported `tool_choice` value must be updated.

SDK users should use [`examples/chat/main.go`](examples/chat/main.go) as the
migration reference. Its `streamModelTurn`, `appendToolCalls`, and `callTools`
functions demonstrate streaming tool-call deltas, preserving the assistant
tool-call message, executing tools, and appending matching tool results before
the next model turn.

Chat Completions accepts these string values:

```json
"tool_choice": "none"
"tool_choice": "auto"
"tool_choice": "required"
```

To force a specific Chat Completions function, replace the old bare function
name:

```json
"tool_choice": "get_weather"
```

with the OpenAI function object:

```json
"tool_choice": {
  "type": "function",
  "function": {
    "name": "get_weather"
  }
}
```

The Responses API uses its flat forced-function form instead:

```json
"tool_choice": {
  "type": "function",
  "name": "get_weather"
}
```

The selected function must exist in the request's `tools` array. The
`required` mode requires at least one function tool. Invalid modes, malformed
function objects, and forced functions that were not declared now return a
request validation error instead of being handled on a best-effort basis.

`tool_choice: "none"` now withholds tools from prompt rendering and suppresses
structured tool-call output. Forced selection limits prompt rendering to the
selected function. Actual selection still depends on the model and chat
template honoring the requested mode.

### Streaming tool calls

Streaming clients must aggregate `tool_calls` deltas rather than expecting the
terminal chunk to contain the completed call:

1. Kronk emits an activity delta once the function name is known.
2. Kronk emits completed arguments in a nonterminal tool-call delta.
3. Kronk emits an empty terminal delta with
   `finish_reason: "tool_calls"`.

Consumers should merge calls by their tool-call `index` and preserve the
reported `id`. Execute a function only after its arguments form valid JSON and
the stream reports the terminal `tool_calls` finish reason.

Tool-call arguments remain JSON encoded inside the `function.arguments`
string. Decode that string before invoking the function. After execution,
append both the assistant message containing `tool_calls` and a `role: "tool"`
message with the matching `tool_call_id` to the next request.

Additional SDK and wire changes:

- Completed, non-streaming `tool_calls` no longer include `index`. Use array
  order for completed calls and use `index` only to merge streaming deltas.
- A nil `ToolCallArguments` value now marshals as the JSON string `"{}"`
  instead of an empty string.
- `ResponseResponse.ToolChoice` changed from `string` to `any`. SDK consumers
  must type-switch between the string modes and the forced-function object.

## Usage Changes

Streaming usage is now opt-in. Previously, streaming responses included usage
by default. Clients that need final token accounting must now request it:

SDK users should follow the streaming loop in
[`examples/chat/main.go`](examples/chat/main.go). The example requests usage,
checks for an empty `Choices` slice before indexing it, captures the separate
usage response, and passes the resulting `*model.Usage` to `printUsage`.

```json
{
  "stream": true,
  "stream_options": {
    "include_usage": true
  }
}
```

When `include_usage` is false or omitted, streaming chunks omit usage. When it
is true:

- normal completion chunks contain `"usage": null`;
- the terminal choice is followed by one additional usage chunk;
- that usage chunk has an empty `choices` array and the final usage totals;
- the usage chunk is emitted before the SSE `[DONE]` marker.

This affects both HTTP streaming and SDK consumers of `ChatStreaming`. Code
that currently indexes `Choices[0]` for every streamed response must first
check `len(Choices)`. A response with no choices and non-nil `Usage` is the
final usage event.

Non-streaming Chat Completions continue to include usage in the final response.
Responses API streams likewise omit final usage unless
`stream_options.include_usage` is true.

Token accounting now follows the generated-token total more closely:

- `completion_tokens` includes reasoning tokens, control tokens, and buffered
  tool-call syntax even when those bytes are not exposed as assistant text;
- `completion_tokens_details.reasoning_tokens` reports the reasoning subset;
- `total_tokens` is `prompt_tokens + completion_tokens`;
- Responses API `output_tokens` follows the same generated-output accounting.

Consumers using these values for billing, quotas, or historical comparisons
should expect totals to change for reasoning and tool-calling requests.

The Go SDK `model.Usage` shape also changed:

| Removed field | Replacement |
| ------------- | ----------- |
| `Usage.OutputTokens` | `Usage.CompletionTokens` |
| `Usage.ReasoningTokens` | `Usage.CompletionTokensDetails.ReasoningTokens` |

The corresponding top-level JSON properties `output_tokens` and
`reasoning_tokens` were removed from Chat Completions usage. Reasoning usage is
now reported at `completion_tokens_details.reasoning_tokens`. If an application
needs the visible, non-reasoning completion count, calculate
`CompletionTokens - CompletionTokensDetails.ReasoningTokens`, recognizing that
the completion total can also contain generated control and tool-call syntax.

The nonstandard `return_prompt` request parameter and the Go SDK
`ChatResponse.Prompt` field were removed. Applications that need to retain a
rendered or source prompt must record it before submitting the request. The
exported `ChatResponseErr` helper also removed its `prompt` argument.

## Network Binding Changes

The default server bind addresses changed from all network interfaces to
loopback:

| Listener | Previous default | New default |
| -------- | ---------------- | ----------- |
| Main API and BUI | `0.0.0.0:11435` | `127.0.0.1:11435` |
| Debug, metrics, and profiling | `0.0.0.0:11445` | `127.0.0.1:11445` |

Local clients continue to work. LAN clients, containers, Kubernetes services,
and remote monitoring systems will no longer reach a default configuration.
Publishing a container port is not sufficient when Kronk is listening only on
the container's loopback interface.

Explicitly restore external listeners where required:

```shell
KRONK_WEB_API_HOST=0.0.0.0:11435
KRONK_WEB_DEBUG_HOST=0.0.0.0:11445
```

Expose the debug listener only on trusted networks. It includes operational
metrics and profiling endpoints.

## HTTP Error Response Changes

Application errors now use an OpenAI-style nested envelope. Clients that
decode error bodies must update from:

```json
{
  "code": "invalid_argument",
  "message": "..."
}
```

to:

```json
{
  "error": {
    "message": "...",
    "type": "invalid_request_error",
    "code": "invalid_argument"
  }
}
```

This applies to validation, authentication, authorization, not-found,
capacity, rate-limit, and other application errors. The HTTP status remains
the primary success/failure signal.

If Chat Completions fails after SSE headers are committed, the stream now
contains an `{"error": {...}}` event. Streaming clients must recognize this
error object independently of normal events containing `choices`; they should
no longer expect every streaming failure to arrive as a choice with
`finish_reason: "error"`.

## Session Storage Changes

The built-in disk-backed IMC session store was removed. The server no longer
supports this model configuration:

```yaml
session-store-kind: disk
session-store-dir: /var/lib/kronk
```

Remove `session-store-dir` and use the default RAM store or set:

```yaml
session-store-kind: ram
```

For direct SDK use, these APIs were removed:

- `model.Config.SessionStoreKind`
- `model.Config.SessionStoreDir`
- `model.SessionStoreKindRAM`
- `model.SessionStoreKindDisk`
- `sdk/kronk/kvstorage/disk`

Custom storage is now injected with `model.WithSessionStoreFactory`. Implement
`kvstorage.Store` and return an independent store from each factory call. See
[`examples/session-store/main.go`](examples/session-store/main.go) for the SDK
pattern.

Old disk session files were temporary process-owned KV snapshots rather than
durable resumable conversations. No data conversion is required; stale
`kronk-sess-*.kv` files can be removed after the old server is stopped.

## Go SDK Changes

### VRAM AutoFit

`vram.AutoFit` now returns a fourth value reporting whether a verified fitting
placement was found:

```go
gpuLayers, expertLayers, result, fits := vram.AutoFit(input, constraints)
if !fits {
    // No placement was verified against the supplied capacity.
}
```

Existing three-result assignments will no longer compile. Explicit CPU-only
placement now uses `gpuLayers == -1`; zero is not the CPU-only sentinel.

### MoE mode

`model.MoEMode` changed from an open string type to a registered enum value.
Predefined values retain the same serialized strings, but callers can no
longer convert arbitrary strings directly:

```go
mode, err := model.ParseMoEMode(value)
// Or use model.MoEModeExpertsCPU and the other predefined values.
```

Replace custom `const` declarations with package values or parsed values.
Existing valid YAML strings remain unchanged; unknown values are now rejected
during decoding.

### Model information

The heuristic `IsGPTModel` field was removed from both
`model.ModelInfo` and `models.ModelInfo`. The native
`GET /v1/kronk/models/{model}` response also no longer contains `is_gpt`.
Consumers should use actual model capabilities instead of inferring behavior
from `gpt` appearing in a model filename.

### Model integrity API

`model.CheckModel` and `model.RemoveVerifiedSentinel` were removed. Filesystem,
digest-sidecar, and verification-record ownership now belongs to
`sdk/tools/models`. Most applications should use that package's model
management and integrity APIs.

Low-level callers can use `model.VerifyArtifact`, but must provide the parsed
`ArtifactDigest` and prior `ArtifactVerification` and must persist returned
verification state themselves.

### Configured sampling seeds

When a request omits `seed`, Kronk now honors a seed configured in
`model.Config.DefaultParams` or `sampling-parameters.seed`. Previously that
configured default was discarded and the request used random sampler state.

To retain random sampling, leave the configured seed unset. An explicit
request seed continues to override the configured default.
