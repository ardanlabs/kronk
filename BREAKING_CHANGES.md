# Breaking Changes by Version

## Index

- [v1.32.4](#v1324)
  - [IMC Session Capacity Changes](#v1324-imc-session-capacity-changes)
  - [Docker Observability Changes](#v1324-docker-observability-changes)
- [v1.31.6](#v1316)
  - [Go Toolchain Changes](#v1316-go-toolchain-changes)
- [v1.31.4](#v1314)
  - [Model Identifier Changes](#v1314-model-identifier-changes)
- [v1.31.2](#v1312)
  - [Model Batch Configuration Changes](#v1312-model-batch-configuration-changes)
- [v1.30.5](#v1305)
  - [Pool TTL Changes](#v1305-pool-ttl-changes)
- [v1.30.4](#v1304)
  - [Go SDK Streaming Changes](#v1304-go-sdk-streaming-changes)
  - [Request Validation Changes](#v1304-request-validation-changes)
  - [Tool Parsing Changes](#v1304-tool-parsing-changes)
  - [Log Probability Changes](#v1304-log-probability-changes)
- [v1.30.3](#v1303)
  - [Auth Changes](#v1303-auth-changes)
  - [Tool Calling Changes](#v1303-tool-calling-changes)
  - [Usage Changes](#v1303-usage-changes)
  - [Network Binding Changes](#v1303-network-binding-changes)
  - [HTTP Error Response Changes](#v1303-http-error-response-changes)
  - [Session Storage Changes](#v1303-session-storage-changes)
  - [Go SDK Changes](#v1303-go-sdk-changes)

## v1.32.4

### v1.32.4: IMC Session Capacity Changes

The default Incremental Message Cache (IMC) session capacity now matches the
generation admission capacity:

```text
# Before
max(nseq-max, 1) × max(3, queue-depth)

# After
max(nseq-max, 1) × queue-depth
```

With `nseq-max: 2` and the default `queue-depth: 2`, Kronk now retains four
working IMC sessions and four System cache entries instead of six of each.
This does not reduce execution concurrency or request admission capacity, but a
workload with more than four active conversation branches may evict reusable
state sooner and perform additional prompt prefill.

Set `imc-session-capacity` explicitly when the desired warm conversation
working set is larger than the new default. For example, use
`imc-session-capacity: 6` to preserve the previous default for two slots and a
queue depth of two. An explicit value must be at least
`nseq-max × queue-depth`.

### v1.32.4: Docker Observability Changes

The bundled Docker Compose observability stack now uses Grafana Alloy instead
of Promtail to collect container logs for Loki. Custom commands, Compose
overrides, automation, or monitoring that refer to the `promtail` service or
container must use `alloy` instead. The configuration directory changed from
`zarf/docker/promtail` to `zarf/docker/alloy`.

The Compose volume `promtail-positions` was replaced by `alloy-data`. Existing
Promtail position data is not migrated or reused. The standard `make
grafana-up` and `make grafana-down` commands already use Alloy and require no
manual changes.

## v1.31.6

### v1.31.6: Go Toolchain Changes

Kronk now requires Go 1.27 or later. Go SDK consumers and users installing
Kronk with `go install` must upgrade their Go toolchain before updating to this
release. Consumers that need to remain on Go 1.26 must use Kronk v1.31.5 or
earlier.

Go 1.27 changes the implementation beneath the existing `encoding/json`
package to use JSON v2 internally while preserving the v1 API and semantics.
Kronk continues to import `encoding/json`; consumers do not need to change
their JSON imports. Exact JSON error text can differ, and unmarshalling is
generally faster. Go 1.27 temporarily supports building with
`GOEXPERIMENT=nojsonv2` to diagnose compatibility problems with the new
implementation.

Kronk now uses Go 1.27's standard-library `uuid` package instead of
`github.com/google/uuid` for its internal identifiers. This does not change
the textual UUID format exposed by Kronk APIs.

Go 1.27 requires macOS 13 Ventura or later. Kronk release binaries built with
Go 1.27 no longer support older macOS releases. The Nix flake also no longer
supports `x86_64-darwin`, which Nixpkgs 26.11 has dropped. The macOS `amd64`
release archive remains available for Intel Macs running macOS 13 or later.

## v1.31.4

### v1.31.4: Model Identifier Changes

Kronk model identifiers now use the canonical `provider/modelID` form on all
public model-facing APIs. Bare model IDs are rejected instead of being searched
across a provider list:

```text
# Before
Qwen3-8B-Q8_0

# After
Qwen/Qwen3-8B-Q8_0
```

Update the `model` field sent to Chat Completions, Responses, Messages,
Embeddings, Reranking, Tokenize, and other inference endpoints. Also update
model IDs supplied to model-management APIs and CLI commands. Discovery
responses such as `GET /v1/models`, and inference responses that carry a model
field, now report the canonical ID and should be treated as the source of truth.

Keys in `~/.kronk/models/model_config.yaml` must also be canonical. A legacy
bare key prevents the server pool from initializing:

```yaml
version: 1
models:
  # Before
  Qwen3-8B-Q8_0:
    context-window: 131072

  # After
  Qwen/Qwen3-8B-Q8_0:
    context-window: 131072
```

Named configurations now append the profile after the canonical base ID. For
example, replace `Qwen3-8B-Q8_0/AGENT` with:

```yaml
models:
  Qwen/Qwen3-8B-Q8_0/AGENT:
    context-window: 131072
```

Clients select that configuration by sending the complete
`Qwen/Qwen3-8B-Q8_0/AGENT` value. The profile changes runtime settings but
continues to use the files installed for `Qwen/Qwen3-8B-Q8_0`.

Model pull and catalog resolution no longer walk a configured provider list
for a bare ID. Use the canonical ID shown by `kronk catalog list --local`, or
continue using an accepted explicit source such as a Hugging Face URL or
`owner/repository/file.gguf`.

## v1.31.2

### v1.31.2: Model Batch Configuration Changes

The separate user-configurable `nbatch` and `nubatch` settings were replaced
by one prompt-processing setting:

```yaml
# Before
nbatch: 8192
nubatch: 2048

# After
prefill-batch-size: 2048
```

`prefill-batch-size` defaults to 2048 and limits how many prompt tokens the
current prefill owner contributes to one decode iteration. Kronk now derives
llama.cpp's internal logical `NBatch` and physical `NUBatch` capacities from
that value, the slot count, and the generation mode. Non-MTP reserves one
generation row per slot. MTP reserves `1 + ndraft` rows per slot and uses one
physical batch for the complete prefill-plus-generation tray.

Direct Go SDK consumers must replace these removed APIs:

| Removed | Replacement |
| ------- | ----------- |
| `model.Config.PtrNBatch` | `model.Config.PtrPrefillBatchSize` |
| `model.Config.PtrNUBatch` | `model.Config.PtrPrefillBatchSize` |
| `model.Config.NBatch()` | `model.Config.PrefillBatchSize()` for configuration; `EffectiveNBatch()` for diagnostics |
| `model.Config.NUBatch()` | `model.Config.PrefillBatchSize()` for configuration; `EffectiveNUBatch()` for diagnostics |
| `model.WithNBatch(...)` | `model.WithPrefillBatchSize(...)` |
| `model.WithNUBatch(...)` | `model.WithPrefillBatchSize(...)` |

The Playground request field changed from `nbatch` and `nubatch` to
`prefill_batch_size`. The BUI playground and configuration sweeps now expose
only **Prefill Batch Size**. Effective `NBatch / NUBatch` values remain visible
as read-only runtime diagnostics on the Slots screen.

When migrating an old configuration with different values, use the old
`nubatch` value as the initial `prefill-batch-size`, then benchmark the workload.
A larger value can finish long-prompt prefill in fewer decode calls, but each
call takes longer before already-generating slots can run again and requires a
larger compute buffer.

## v1.30.5

### v1.30.5: Pool TTL Changes

A pool TTL of zero now disables idle model expiration. Previously, the Kronk
and Bucky Go SDK pools silently replaced zero or negative TTL values with a
five-minute default. Negative TTL values now return a configuration error.

Direct SDK consumers that relied on a zero-valued `pool.Config.TTL`,
`kronk/pool.Config.TTL`, or `bucky/pool.Config.TTL` selecting the old default
must now set a positive duration explicitly. The server default remains 20
minutes when `KRONK_POOL_TTL` and `--pool-ttl` are omitted; set either one to
`0` to disable idle expiration.

Disabling idle expiration does not pin a model. Loaded models can still be
evicted to satisfy the pool-count limit or memory budget, and explicit unload,
server shutdown, and process termination continue to remove them.

## v1.30.4

### v1.30.4: Go SDK Streaming Changes

The low-level `model.(*Model).ChatStreaming` method now returns a startup error
in addition to the response channel:

```go
// Before
ch := mdl.ChatStreaming(ctx, request)

// After
ch, err := mdl.ChatStreaming(ctx, request)
if err != nil {
	// The request failed validation before streaming started.
}
```

Existing callers that assign one result, range directly over the method call,
use its method expression, or implement an interface containing the old
signature will no longer compile.

Request validation now runs before the response channel is created. Validation
failures are returned directly as `nil, err`; they are no longer delivered as
an error `model.ChatResponse` on the channel. Errors that happen after startup
continue to be reported through the stream.

The higher-level `kronk.(*Kronk).ChatStreaming` and
`kronk.(*Kronk).ResponseStreaming` signatures did not change because they
already returned `(channel, error)`. Callers of those methods should continue
checking the returned error before reading the channel.

Streaming responses now report `delta.role: "assistant"` once, in an initial
empty-content delta. Subsequent text and tool-call deltas omit `role`.
Previously, each text or tool-call delta repeated the assistant role. Consumers
must retain the role from the initial delta and must not discard later deltas
merely because their `ResponseMessage.Role` or JSON `delta.role` is empty.

### v1.30.4: Request Validation Changes

Invalid model request parameters now consistently wrap
`model.ErrInvalidRequest`. This includes parameter parsing, invalid sampling
seeds, unsupported response formats or grammars, and invalid tokenize input.
Direct SDK consumers should classify these errors with:

```go
if errors.Is(err, model.ErrInvalidRequest) {
	// Correct the request rather than retrying it unchanged.
}
```

This changes observable error strings and changes `errors.Is` from false to
true for affected errors. Integrations that map `ErrInvalidRequest` to a
protocol status now classify these failures as client errors; the Kronk HTTP
server returns HTTP 400 for them instead of treating them as internal model
errors.

The Chat Completions `n` parameter now supports only the value `1`. Omitted or
null `n` remains valid. Requests that previously supplied zero, a negative or
fractional value, a string value, or a numeric value greater than one now fail
with `model.ErrInvalidRequest`. Kronk does not generate multiple choices in one
request; clients must submit separate requests when multiple samples are
needed.

### v1.30.4: Tool Parsing Changes

Tool-call parsing is now strict and atomic in these exported parser packages:

- `sdk/kronk/parsers/deepseek`
- `sdk/kronk/parsers/gemma`
- `sdk/kronk/parsers/glm`
- `sdk/kronk/parsers/gpt`
- `sdk/kronk/parsers/kimi`
- `sdk/kronk/parsers/lfm`
- `sdk/kronk/parsers/llama`
- `sdk/kronk/parsers/mistral`
- `sdk/kronk/parsers/qwen`
- `sdk/kronk/parsers/toolcall`

Malformed or mixed output that was previously repaired, partially accepted, or
reduced to its valid calls is now rejected as one failed
`model.ResponseToolCall`. The failed result has `Status == 2`, retains the
original model output in `Raw`, and describes the parse failure in `Error`.
Newly rejected forms include duplicate JSON keys or XML parameters, junk around
or between calls, malformed siblings after a valid call, missing wrappers or
delimiters, non-object arguments, and structural JSON repairs.

Consumers must check every returned tool call's `Status` before execution and
must not execute a valid-looking subset when the parser reports an atomic
failure. Applications that intentionally accept nonstandard model output must
normalize and validate it outside these parsers.

The state machines returned by the `mistral` and `toolcall` parsers no longer
implement `model.ToolCallDeltaStreamer`. Dynamic assertions to that optional
interface now return false, and those parser families no longer emit
provisional tool-call identity/name deltas while arguments are still being
generated. Streaming consumers must wait for the completed tool-call delta and
the terminal `finish_reason: "tool_calls"` before execution.

When generation reaches the token limit with incomplete or invalid tool-call
syntax, Kronk now preserves the raw truncated text as assistant content and
removes failed tool-call entries. Previously that output could remain on the
tool channel or be represented as failed `tool_calls`. Streaming and
non-streaming consumers must therefore allow a length-limited response to
contain ordinary assistant content that resembles an unfinished tool call.

Parser end-of-generation and flush handling is also lossless and may expose
malformed trailing output that older parsers silently discarded. This is part
of the strict atomic behavior above; callers should treat the parser's failed
result as authoritative rather than attempting to execute an earlier call from
the same model output.

### v1.30.4: Log Probability Changes

When `logprobs` is enabled, `choices[].logprobs.content` no longer includes an
entry for the terminal vocabulary end-of-generation token. The terminal token
does not produce response content, so each log-probability entry now aligns
with an emitted output token.

Consumers that expected one extra terminal entry or compared the logprobs
array length with raw generated-token accounting must remove that assumption.
This does not change sampling and does not affect requests with
`logprobs: false`.

## v1.30.3

### v1.30.3: Auth Changes

Kronk now uses one setting to describe API access:

```shell
KRONK_AUTHORIZATION_MODE=<mode>
```

#### Choose the intended mode

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

#### Recommended mapping from the old settings

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

#### Remove the legacy settings

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

#### Embedded authentication

When `KRONK_AUTH_HOST` is empty, Kronk runs the embedded authentication service. The selected authorization mode automatically configures it; do not set `KRONK_AUTH_LOCAL_ENABLED`.

Example:

```shell
KRONK_AUTHORIZATION_MODE=full-protected
KRONK_WEB_ADMIN_ENABLED=true
```

For protected BUI login, retain the configured admin password digest:

```shell
KRONK_WEB_ADMIN_PASSWORD_SHA_256=<sha256>
```

#### External authentication

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

##### External auth and the BUI

Protected BUI password login currently uses Kronk’s embedded security store. Therefore, when using an external auth host with `management`, `authenticated`, or `full-protected`, disable the BUI:

```shell
KRONK_WEB_ADMIN_ENABLED=false
```

API clients can continue using bearer JWTs issued by the external auth service.

#### Suggested production configuration

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

#### Post-deployment checks

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

### v1.30.3: Tool Calling Changes

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

#### Streaming tool calls

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

### v1.30.3: Usage Changes

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

| Removed field           | Replacement                                     |
| ----------------------- | ----------------------------------------------- |
| `Usage.OutputTokens`    | `Usage.CompletionTokens`                        |
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

### v1.30.3: Network Binding Changes

The default server bind addresses changed from all network interfaces to
loopback:

| Listener                      | Previous default | New default       |
| ----------------------------- | ---------------- | ----------------- |
| Main API and BUI              | `0.0.0.0:11435`  | `127.0.0.1:11435` |
| Debug, metrics, and profiling | `0.0.0.0:11445`  | `127.0.0.1:11445` |

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

### v1.30.3: HTTP Error Response Changes

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

### v1.30.3: Session Storage Changes

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

### v1.30.3: Go SDK Changes

#### VRAM AutoFit

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

#### MoE mode

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

#### Model information

The heuristic `IsGPTModel` field was removed from both
`model.ModelInfo` and `models.ModelInfo`. The native
`GET /v1/kronk/models/{model}` response also no longer contains `is_gpt`.
Consumers should use actual model capabilities instead of inferring behavior
from `gpt` appearing in a model filename.

#### Model integrity API

`model.CheckModel` and `model.RemoveVerifiedSentinel` were removed. Filesystem,
digest-sidecar, and verification-record ownership now belongs to
`sdk/tools/models`. Most applications should use that package's model
management and integrity APIs.

Low-level callers can use `model.VerifyArtifact`, but must provide the parsed
`ArtifactDigest` and prior `ArtifactVerification` and must persist returned
verification state themselves.

#### Configured sampling seeds

When a request omits `seed`, Kronk now honors a seed configured in
`model.Config.DefaultParams` or `sampling-parameters.seed`. Previously that
configured default was discarded and the request used random sampler state.

To retain random sampling, leave the configured seed unset. An explicit
request seed continues to override the configured default.
