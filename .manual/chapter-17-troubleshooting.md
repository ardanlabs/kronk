# Chapter 17: Troubleshooting

## Table of Contents

- [17.1 Start with Diagnostics](#171-start-with-diagnostics)
- [17.2 Libraries and Devices](#172-libraries-and-devices)
- [17.3 Models, Catalog, and Storage](#173-models-catalog-and-storage)
- [17.4 Memory and Performance](#174-memory-and-performance)
- [17.5 Requests and Streaming](#175-requests-and-streaming)
- [17.6 Authentication](#176-authentication)
- [17.7 IMC](#177-imc)
- [17.8 MCP](#178-mcp)
- [17.9 Ports, Processes, and Permissions](#179-ports-processes-and-permissions)
- [17.10 Reporting a Problem](#1710-reporting-a-problem)

---

This chapter is a symptom-first guide to common failures. For configuration
details, follow the links to the chapter that owns that subsystem.

### 17.1 Start with Diagnostics

Run the built-in diagnostic before changing configuration:

```shell
kronk diagnose
```

It reports Kronk and yzma versions, host hardware, the active llama.cpp
installation, detected compute devices, and a small benchmark. It does not
download anything unless `--install` is supplied. Useful variants are:

```shell
kronk diagnose --no-bench
kronk diagnose --format json
kronk diagnose --format yaml
```

Run the server in the foreground to see JSON logs on stdout:

```shell
kronk server start
```

For a server started with `-d`, follow its log file with:

```shell
kronk server logs
```

`--insecure-logging` includes prompts, responses, and detailed model
configuration. Use it only for local diagnosis because the output can contain
sensitive data. `--llama-log 1` enables lower-level llama.cpp messages;
`--llama-log 0` disables them.

The main API exposes two unauthenticated process health checks:

```shell
curl http://localhost:11435/v1/liveness
curl -i http://localhost:11435/v1/readiness
```

Readiness currently returns an empty `200 OK` when the HTTP service is running.
It does not verify libraries, devices, memory, loaded models, or inference.
Metrics and profiles are served from the unauthenticated debug server; see
[Chapter 15](chapter-15-observability.md#151-debug-and-health-endpoints) before
exposing that port.

### 17.2 Libraries and Devices

#### `unable to load library`

Install the bundle selected for the current operating system, architecture,
and processor:

```shell
kronk libs --local
kronk devices
```

If detection selected the wrong backend, install explicitly:

```shell
KRONK_PROCESSOR=metal kronk libs --local
KRONK_PROCESSOR=cuda kronk libs --local
KRONK_PROCESSOR=rocm kronk libs --local
KRONK_PROCESSOR=vulkan kronk libs --local
KRONK_PROCESSOR=cpu kronk libs --local
```

Not every combination is published. Check the live matrix with
`kronk libs --list-combinations`. See
[Chapter 2: Libraries](chapter-02-installation.md#24-libraries) for installation
and path details.

#### NVIDIA is visible but llama.cpp uses the CPU

`nvidia-smi` proves that the driver is available, but a native CUDA bundle also
needs the CUDA runtime libraries against which it was linked. On Linux, find
the active library path in `kronk diagnose`, then inspect the backend for
unresolved dependencies:

```shell
ldd <lib-path>/libggml-cuda.so | grep -iE 'not found|cudart|cublas'
```

Install the matching CUDA runtime packages for the bundle and operating system.
For containers, use the current `latest-cuda` image and grant GPU access with
`--runtime=nvidia --gpus all`; the required runtime libraries are included in
that image.

#### A library update introduced crashes or bad output

The normal CLI installs Kronk's pinned default. `--upgrade` and the server's
`--allow-upgrade=true` opt into newer llama.cpp releases. List installed
bundles and pin a known-good version when investigating a regression:

```shell
kronk libs --list-installs
kronk libs --local --version=b5490
kronk server start --lib-version=b5490
```

Unset `KRONK_LIB_VERSION` after the pinned default contains the required fix.
Libraries are not hot-reloaded; restart the server after switching
`KRONK_LIB_PATH`.

### 17.3 Models, Catalog, and Storage

#### A downloaded model is not listed

Kronk stores data beneath `KRONK_BASE_PATH`, which defaults to `~/.kronk`.
Inspect the model index rather than relying on a hard-coded directory:

```shell
kronk model list --local
kronk model index --local
```

Indexing scans model files and checks available size and SHA metadata. An
arbitrary GGUF without corresponding checksum metadata cannot receive the same
integrity validation as a model downloaded by Kronk.

#### A model is missing, incomplete, or corrupt

Pull it again using any supported source form:

```shell
kronk model pull <model-id> --local
```

`model pull` checks the catalog and automatically walks configured providers
when an ID has not been resolved before. A separate `model resolve` step is not
normally required. Interrupted downloads are resumable; if a file remains
invalid, remove that model through Kronk and pull it again:

```shell
kronk model remove <model-id> --local
kronk model pull <model-id> --local
```

For a gated or private Hugging Face repository, provide a read token:

```shell
export KRONK_HF_TOKEN=hf_xxx
kronk model pull <model-id> --local
```

#### `catalog.yaml` was hand-edited and no longer parses

The default catalog is `<base>/catalog/catalog.yaml`. Restore valid YAML from a
backup before running catalog commands; those commands must parse the file and
cannot repair malformed YAML. Do not use `catalog remove` as a syntax-repair
tool because it also removes the selected model's downloaded files.

Catalog administration is covered in
[Chapter 8](chapter-08-model-server.md#86-catalog-operations).

### 17.4 Memory and Performance

#### `unable to init context` or `unable to get memory`

The model, runtime buffers, and configured context do not fit. Change one
variable at a time:

1. Reduce `context-window`.
2. Reduce `nseq-max`.
3. Use a quantized KV cache such as `q8_0`.
4. Move KV state or model layers to CPU.
5. Choose a smaller or more heavily quantized GGUF.

Use the BUI VRAM Calculator for a model-specific estimate and retain headroom.
See [Chapter 3: Memory Planning](chapter-03-model-configuration.md#36-memory-planning-and-quantization).

#### `input tokens [N] exceed context window [M]`

The rendered prompt is already larger than the configured context. Shorten the
conversation or system prompt, or increase `context-window` if memory permits.
Cached prefix tokens still consume context capacity.

#### `the context window is full`

Input plus generated tokens exhausted the context during inference. Request
fewer output tokens, shorten the input, or increase the context. YaRN may extend
supported RoPE models, but it is not a generic memory fix; follow
[Chapter 7](chapter-07-yarn-extended-context.md).

#### Slow inference or slow time to first token

Start with `kronk diagnose` and confirm that llama.cpp sees the expected GPU.
A cold request includes model loading, and a large uncached prompt includes
prefill. Partial CPU offload can reduce token throughput. Compare representative
requests after the model is warm rather than relying on the first request.

Use Chapter 15's request, queue, prefill, TTFT, token-rate, and pool metrics to
separate loading, waiting, prompt processing, and generation. IMC-specific
diagnosis is below.

### 17.5 Requests and Streaming

#### `context deadline exceeded`

The source of the deadline matters:

- a client or reverse proxy may cancel first;
- chat handlers impose a 180-minute request context deadline;
- the HTTP server defaults to a 30-second read timeout and a 60-minute write
  timeout.

`--read-timeout` covers reading the request, not model execution. Increase
`--write-timeout` only when a long response is being cut off by the server:

```shell
kronk server start --write-timeout 90m
```

Check client and proxy timeouts separately. Large prompts and queued requests
should be diagnosed with the timing metrics rather than masking them with a
larger HTTP read timeout.

#### A stream stops or does not parse

OpenAI-compatible chat streaming uses SSE records of the form:

```text
data: {"id":"...","choices":[...]}

data: [DONE]
```

Kronk sends an SSE comment every 15 seconds as a keepalive. Clients must ignore
comment lines and handle normal `finish_reason` values. `/v1/messages` uses
named `event:` records instead of the OpenAI chat format.

A missing `[DONE]` commonly means the client disconnected, a proxy timed out,
or the server encountered an error. Correlate the request with its `trace_id`
in the JSON logs.

### 17.6 Authentication

HTTP clients receive a generic authentication failure. The server's JSON log
contains the specific cause, commonly one of these:

- no authorization header — add a bearer token;
- `invalid token:` — the JWT is malformed, expired, or signed by an unknown
  key;
- `not authorized:` — the token lacks the endpoint grant;
- `rate limit exceeded:` — the grant's current window is exhausted.

Use the generated master token for administration on a default local setup:

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
```

Create a replacement user token with only the required grants:

```shell
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings,rerank,responses,messages,tokenize,transcriptions
```

Rate limits use forms such as `chat-completions:10000/day`. Token creation,
key rotation, and production hardening are covered in
[Chapter 12](chapter-12-security-authentication.md).

### 17.7 IMC

IMC is enabled by default. It externalizes cached session state to RAM by
default or to the configured disk session store. See
[Chapter 5](chapter-05-message-caching.md) for its lifecycle and settings.

#### Every turn rebuilds the cache

Common causes are changed earlier messages, changed template inputs, a prompt
below `cache-min-tokens`, or cache pressure. Relevant JSON log statuses include
`session[N] mismatch`, `sys-prompt-match`, `token prefix match found`,
`no usable token prefix match`, and `kv-pressure-evict`.

Keep earlier conversation messages stable and use a deterministic template.
Increase `nseq-max` only when additional inference concurrency and its memory
cost are both appropriate; IMC maintains more session identities than active
decode slots.

#### `server busy processing other requests, try again shortly`

No IMC session was available. Depending on the planning path, Kronk may return
this immediately or after waiting up to `cache-slot-timeout`. It is a transient
request failure: wait and retry from the client. If it is frequent, inspect
long-running requests and queue/cache metrics before increasing `nseq-max`.
Increasing `cache-slot-timeout` affects only paths that wait for a session.

#### `imc restore failed` or `imc extend stale`

The current request fails. Retry it from the client; the server does not
automatically repeat the request. Repeated restore failures warrant checking
memory pressure, the session-store configuration, and nearby low-level errors.
Reducing `context-window` or concurrency can lower memory pressure.

### 17.8 MCP

The endpoint is `http://localhost:9000/mcp` without a trailing slash and uses
Streamable HTTP. Common failures are:

- **404 after a server restart:** discard the stale in-memory session ID and
  initialize a new MCP session.
- **Brave authentication failure:** set `KRONK_MCP_BRAVE_API_KEY` for embedded
  mode or `MCP_MCP_BRAVE_API_KEY` for standalone mode before startup.
- **Unknown `kronk_fuzzy_edit`:** with the shipped OpenCode configuration, the
  exposed names are `kronk_fuzzy_edit` and `kronk_web_search`.
- **`old_string not found`:** read the current file and provide one unique
  block; the same error also covers ambiguous matches.
- **Embedded server absent:** `KRONK_MCP_ENABLED=false` or a non-empty
  `KRONK_MCP_HOST` disables it. The host setting does not configure a proxy or
  client connection.
- **401 Unauthorized:** when MCP authentication is enabled, send the same
  Kronk admin bearer token on every request, including session initialization
  and notifications. Inference-scoped application tokens are not accepted.

MCP authentication is disabled by default, and `fuzzy_edit` has the process's
filesystem access. Keep it on loopback unless bearer authentication, TLS, and
network restrictions are configured. See
[Chapter 16](chapter-16-mcp-service.md) for configuration and the complete
handshake.

### 17.9 Ports, Processes, and Permissions

Default listeners are `11435` for the API, `11445` for model-server debugging,
and `9000` for embedded MCP. Standalone MCP also starts a debug listener on
`9010`. Find a conflicting process before changing ports:

```shell
lsof -nP -iTCP:11435 -sTCP:LISTEN
lsof -nP -iTCP:9000 -sTCP:LISTEN
```

Move the API or debug listener with `--api-host` or `--debug-host`. Standalone
MCP uses `MCP_MCP_HOST` and `MCP_WEB_DEBUG_HOST`.

Detached mode stores `kronk.pid` and `kronk.log` under `KRONK_BASE_PATH`. If
`kronk server stop` encounters a stale PID, verify that no Kronk process owns
the API or debug port before removing only the stale PID file.

BadgerDB also permits only one model-server process to use the rate-limit
database. A lock error means another process owns `<base>/badger`; stop that
process. Do not delete Badger's `LOCK` file while a server may be running.

For permission errors, make the selected base path writable by the service
user. The server enforces mode `0700` on `<base>/keys` and `0600` on private
key files. Avoid recursively making credentials readable by other users.

Whisper-specific failures are listed in
[Chapter 18 §18.11](chapter-18-bucky.md#1811-troubleshooting).

### 17.10 Reporting a Problem

Include:

- `kronk diagnose --format json` output, or `--no-bench` if benchmarking fails;
- the Kronk version and relevant JSON log records;
- operating system, architecture, GPU, and driver/runtime versions;
- model ID and non-default configuration;
- the complete error text and reproducible steps; and
- whether the failure occurs on the first request, after warmup, or only under
  concurrency.

Remove tokens, prompts, responses, filesystem secrets, and other sensitive
values before sharing diagnostic output.

---

_Next: [Chapter 18: Bucky (Audio Transcription)](chapter-18-bucky.md)_
