# Chapter 16: Troubleshooting

## Table of Contents

- [16.1 Library Issues](#161-library-issues)
- [16.2 Model Loading Failures](#162-model-loading-failures)
- [16.3 Memory Errors](#163-memory-errors)
- [16.4 Request Timeouts](#164-request-timeouts)
- [16.5 Authentication Errors](#165-authentication-errors)
- [16.6 Streaming Issues](#166-streaming-issues)
- [16.7 Performance Issues](#167-performance-issues)
- [16.8 IMC Caching Issues](#168-imc-caching-issues)
- [16.9 Viewing Logs](#169-viewing-logs)
- [16.10 Common Error Messages](#1610-common-error-messages)
- [16.11 Catalog & Model Pull Issues](#1611-catalog--model-pull-issues)
- [16.12 MCP Service Issues](#1612-mcp-service-issues)
- [16.13 Port Conflicts & Filesystem](#1613-port-conflicts--filesystem)
- [16.14 Getting Help](#1614-getting-help)

---

This chapter covers common issues, their causes, and solutions.

### 16.1 Library Issues

**Error: "unable to load library"**

The llama.cpp shared libraries are missing or incompatible with your hardware.

**Solution:**

```shell
kronk libs --local
```

Or download via the BUI Libraries page.

Kronk auto-detects your GPU hardware and selects the correct library bundle.
If auto-detection fails, set the processor explicitly:

```shell
# For Mac with Apple Silicon
KRONK_PROCESSOR=metal kronk libs --local

# For NVIDIA GPU
KRONK_PROCESSOR=cuda kronk libs --local

# For AMD GPU (ROCm, Linux only)
KRONK_PROCESSOR=rocm kronk libs --local

# For Vulkan (cross-platform, including iGPUs)
KRONK_PROCESSOR=vulkan kronk libs --local

# For CPU only
KRONK_PROCESSOR=cpu kronk libs --local
```

See [Chapter 3: Processor Selection](chapter-03-model-configuration.md#32-processor-selection)
for details on how auto-detection works on each platform.

**Problem: New library version causes crashes or bad output**

The standalone `kronk libs` CLI installs the well-known default version
of llama.cpp by default, which is conservative and changes only when the
Kronk release bumps it. The model server (`kronk server start`) defaults
to `--allow-upgrade=true` and tracks the latest llama.cpp release, so a
long-running server can pick up a regression — crashes during model
loading, decode errors, or degraded output quality. When this happens,
pin the library to a known-good version using `KRONK_LIB_VERSION` (or
`--version` on the CLI).

**Pin to a specific version:**

```shell
# Install a specific version
kronk libs --version=b5490 --local

# Or use the environment variable
KRONK_LIB_VERSION=b5490 kronk libs --local
```

**Start the server with a pinned version:**

```shell
kronk server start --lib-version=b5490
```

Or set it globally so both `kronk libs` and `kronk server start` use the
same version:

```shell
export KRONK_LIB_VERSION=b5490
kronk libs --local
kronk server start
```

**Check your current installed version:**

```shell
kronk libs --version
```

This shows the installed version, architecture, OS, processor, and the
latest available version. The CLI will only upgrade past the installed
version when you pass `--upgrade`; otherwise it sticks to the well-known
default version (or whatever is on disk if it is already newer).

**When to pin:** Pin whenever a new llama.cpp release breaks something
you depend on. Unset `KRONK_LIB_VERSION` once the upstream fix is released
to resume tracking either the default version (CLI) or latest (server with
`--allow-upgrade=true`).

See [Chapter 2: Installing Libraries](chapter-02-installation.md#23-installing-libraries)
for the full compatibility matrix.

**Error: "unknown device"**

The specified GPU device is not recognized by the loaded library.

**Causes:**

- Wrong processor for your hardware (e.g., `cuda` library on a Mac)
- GPU drivers not installed or outdated
- Library/processor mismatch (CPU library loaded but GPU device requested)

**Solution:**

Verify your processor and re-download libraries:

```shell
# Check what Kronk detects
kronk devices

# Re-install matching libraries
kronk libs --local
```

**Problem: "unable to load library" pointing at the wrong folder**

Library bundles now live at `<base>/libraries/<os>/<arch>/<processor>/`,
one folder per `(arch, os, processor)` triple. If `dlopen` reports a path
like `<base>/libraries/libllama.dylib` (libraries directly under the
root), you have an installation from before the per-triple layout. The
SDK migrates the legacy layout into the correct triple folder
automatically on first call to `libs.New()`/`libs.Path()`. If migration
fails, just re-run:

```shell
kronk libs --local
```

The new install lands at `<base>/libraries/<os>/<arch>/<processor>/` and
the runtime resolves to the same folder.

**Problem: Server is loading the wrong install**

To switch the active install (for example to a previously downloaded
CUDA or CPU bundle), point `KRONK_LIB_PATH` at its triple folder and
restart the server. Libraries are not hot-reloaded.

```shell
# List installed bundles
kronk libs --list-installs

# Switch active install
export KRONK_LIB_PATH=~/.kronk/libraries/linux/amd64/cuda
kronk server start
```

If `KRONK_LIB_PATH` points at a directory containing `version.json`,
Kronk uses it as-is. If it points at a non-empty directory without a
`version.json`, Kronk treats it as a read-only user-managed build and
will refuse mutating operations against it (errors will mention
`read-only` or `ErrReadOnly`).

### 16.2 Model Loading Failures

**Error: "unable to load model"**

The model file is missing, corrupted, or incompatible.

**Check model exists:**

```shell
ls ~/.kronk/models/
```

**Re-download the model:**

```shell
kronk model pull <model-id> --local
```

**Problem: Model exists but server says "model not found"**

The model files are on disk but Kronk can't find them. This happens when the
model index (`.index.yaml`) is out of sync — for example after manually
moving model files, a failed download, or removing a model outside of Kronk.

**Solution — rebuild the model index:**

```shell
# With the server running (triggers re-index via API)
kronk model index

# Without the server (rebuilds index directly on disk)
kronk model index --local
```

This scans `~/.kronk/models/`, validates each GGUF file, and rebuilds the
`.index.yaml` that Kronk uses for fast model lookups. You can also trigger
a rebuild from the BUI Models page.

**When to rebuild the index:**

- Model files were moved or renamed manually
- A download was interrupted and left partial files
- `kronk model list` doesn't show a model you know is downloaded
- After deleting model files outside of `kronk model remove`

### 16.3 Memory Errors

**Error: "unable to init context" or "unable to get memory"**

Insufficient memory for the model plus its KV cache at the configured
context window size.

**Causes:**

- Context window too large for available VRAM/RAM
- Too many parallel sequences (`nseq-max`)
- Model weights don't fit in available memory

**Solutions:**

Reduce context window:

```yaml
Qwen/Qwen3-8B-Q8_0:
  context-window: 8192 # Reduce from 32768
```

Reduce parallel sequences:

```yaml
Qwen/Qwen3-8B-Q8_0:
  nseq-max: 1 # Single request at a time
```

Use quantized KV cache:

```yaml
Qwen/Qwen3-8B-Q8_0:
  cache-type-k: q8_0 # ~50% less KV cache memory vs f16
  cache-type-v: q8_0
```

See [Chapter 3: VRAM Estimation](chapter-03-model-configuration.md#39-vram-estimation)
for how to calculate whether a model fits in your hardware.

**Error: "the context window is full"**

The total token count (input + cached + generated) exceeds the configured
context window during inference.

**Solutions:**

- Reduce input size (fewer messages, shorter prompts)
- Increase `context-window` in model config (requires more VRAM)
- Enable YaRN for extended context (see
  [Chapter 6](chapter-06-yarn-extended-context.md))

**Error: "input tokens [N] exceed context window [M]"**

The prompt itself (after tokenization) is larger than the context window,
before any generation can begin.

**Solutions:**

- Shorten the prompt or system message
- Increase `context-window`
- If using IMC, the cached prefix counts toward the limit

### 16.4 Request Timeouts

**Error: "context deadline exceeded"**

The request took longer than the configured HTTP timeout.

**Causes:**

- Large prefill with many input tokens
- Server under heavy load with all slots busy
- Model too slow for the requested output length

**Solutions:**

Increase HTTP timeouts:

```shell
kronk server start \
  --read-timeout 5m \
  --write-timeout 30m
```

Or via environment variables:

```shell
export KRONK_WEB_READ_TIMEOUT=5m
export KRONK_WEB_WRITE_TIMEOUT=30m
```

**Error: "server busy processing other requests, try again shortly"**

All IMC sessions have pending cache builds in-flight, or the slot preemption
timeout was reached.

**Causes:**

- All sessions are busy building caches simultaneously
- A long-running request is occupying the slot pool

**Solutions:**

- Wait and retry the request — the error is transient
- Increase `nseq-max` to allow more concurrent sessions
- Increase `cache-slot-timeout` (default: 30 seconds) if requests need
  more time

### 16.5 Authentication Errors

**Error: "unauthorized: no authorization header"**

Authentication is enabled but no token was provided.

**Solution:**

Include the Authorization header:

```shell
curl http://localhost:11435/v1/chat/completions \
  -H "Authorization: Bearer $(cat ~/.kronk/keys/master.jwt)" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

**Error: "invalid token"**

The token is malformed, expired, or signed with an unknown key.

**Causes:**

- Token has expired (check `--duration` when created)
- Signing key was deleted or rotated
- Token is truncated or corrupted

**Solution:**

Create a new token:

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings
```

**Error: "endpoint not authorized"**

The token doesn't include the requested endpoint in its allowed list.

**Solution:**

Create a new token with the required endpoints:

```shell
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings,rerank,responses,messages
```

**Error: "rate limit exceeded"**

The token has exceeded its configured rate limit.

**Solution:**

Wait for the rate limit window to reset, or create a new token with
higher limits:

```shell
kronk security token create \
  --duration 720h \
  --endpoints "chat-completions:10000/day"
```

### 16.6 Streaming Issues

**Problem: Streaming stops mid-response**

**Causes:**

- Client disconnected (network timeout, browser tab closed)
- HTTP write timeout reached on the server
- Model generated an end-of-generation token (normal completion)

**Solutions:**

- Check if the response includes a `finish_reason` — if it does, the model
  stopped normally
- Increase `--write-timeout` if large responses are being cut off
- Run the server in foreground to see logs:

```shell
kronk server start  # Logs print to stdout
```

**Problem: SSE events not parsing correctly**

Ensure your client handles Server-Sent Events (SSE) format. Each event is
prefixed with `data: ` and terminated by two newlines:

```
data: {"id":"...","choices":[{"delta":{"content":"Hello"}}],...}\n\n
data: [DONE]\n\n
```

### 16.7 Performance Issues

**Problem: Slow time to first token (TTFT)**

**Causes:**

- Large conversation prefix being re-processed from scratch
- IMC not enabled (every request re-processes the full prompt)
- Cold model load on first request

**Solutions:**

Enable IMC to cache the conversation prefix:

```yaml
Qwen3.6-35B-A3B-UD-Q4_K_M/AGENT:
  incremental-cache: true
```

With IMC, only the new message is prefilled — cached tokens are restored
from RAM in ~10-30ms regardless of conversation length.

**Problem: Slow token generation (tokens/second)**

**Causes:**

- Running on CPU instead of GPU
- Model too large for available VRAM (partial CPU offload)
- MoE model on Apple Silicon (scattered memory access patterns)

**Solutions:**

Check GPU is being used:

```shell
# On macOS, check Metal usage
sudo powermetrics --samplers gpu_power

# On Linux with NVIDIA
nvidia-smi
```

Ensure all layers are on GPU (default):

```yaml
Qwen/Qwen3-8B-Q8_0:
  ngpu-layers: 0 # 0 = all layers on GPU (default)
```

For MoE models on Apple Silicon, consider a dense model at lower
quantization — the sequential memory access pattern is faster than MoE's
scattered expert routing (see
[Chapter 3: Model-Specific Tuning](chapter-03-model-configuration.md#310-model-specific-tuning)).

### 16.8 IMC Caching Issues

**Problem: Every request triggers a full cache rebuild**

**Causes:**

- Client is modifying earlier messages between requests
- Non-deterministic Jinja template producing different tokens for the same
  messages
- `nseq-max` too low for the number of concurrent sub-agents (cache
  thrashing)

**Diagnosis:**

Look for these log patterns:

| Log Message                                | Meaning                                       |
| ------------------------------------------ | --------------------------------------------- |
| `session[N] mismatch`                      | Hash changed — messages were modified         |
| `sys-prompt-match`                         | System prompt preserved, conversation rebuilt |
| `token prefix match found`                 | Partial prefix salvaged via token comparison  |
| `no usable token prefix match`             | No salvageable prefix, full rebuild required  |
| `kv-pressure-evict`                        | Stale session evicted to free KV space        |
| `all sessions pending, waiting`            | All sessions busy, request is waiting         |
| `imc-restore-start` / `imc-restore-done`   | KV state being restored from RAM              |
| `imc-snapshot-start` / `imc-snapshot-done` | KV state being snapshotted to RAM             |

**Solutions:**

- Increase `nseq-max` to match the number of concurrent sub-agents
- Check if the client is modifying conversation history between requests
- If using a non-deterministic template, IMC falls back to token prefix
  matching automatically — this is expected behavior

**Problem: IMC restore fails**

**Error:** `imc restore failed for seq N`

The RAM-to-VRAM restore (`StateSeqSetData`) failed for a session.

**Cause:** Usually indicates the KV cache memory could not be allocated
(VRAM pressure from other sessions or models).

**Solution:** The session is automatically reset and the next request
triggers a full rebuild. If this happens frequently, reduce `nseq-max`
or `context-window` to lower VRAM pressure.

### 16.9 Viewing Logs

**Run server in foreground:**

```shell
kronk server start
```

All logs print to stdout with structured key-value format.

**Enable verbose logging:**

```shell
kronk server start --insecure-logging
```

This logs full message content including prompts and responses. Never use
in production — it exposes sensitive conversation data.

**Enable llama.cpp logging:**

```shell
kronk server start --llama-log 1
```

Shows low-level inference engine messages from llama.cpp. Useful for
debugging GPU issues, memory allocation failures, and decode errors.

**Disable llama.cpp logging:**

```shell
kronk server start --llama-log 0
```

### 16.10 Common Error Messages

| Error                                        | Cause                            | Solution                               |
| -------------------------------------------- | -------------------------------- | -------------------------------------- |
| `unable to load library`                     | Missing llama.cpp libraries      | `kronk libs --local`                   |
| `unknown device`                             | Wrong processor for hardware     | Check `kronk devices`, re-install libs |
| `unable to load model`                       | Missing or corrupt model file    | Re-download with `kronk model pull`    |
| `unable to init context`                     | Insufficient VRAM/RAM            | Reduce context-window or nseq-max      |
| `input tokens [N] exceed context window [M]` | Prompt too large                 | Shorten prompt or increase context     |
| `the context window is full`                 | KV cache exhausted during decode | Reduce input size or increase context  |
| `context deadline exceeded`                  | HTTP timeout reached             | Increase `--write-timeout`             |
| `server busy processing other requests`      | All IMC sessions busy            | Retry, or increase nseq-max            |
| `no authorization header`                    | Missing auth token               | Add `Authorization: Bearer <token>`    |
| `invalid token`                              | Expired or malformed JWT         | Create a new token                     |
| `endpoint not authorized`                    | Token missing endpoint scope     | Create token with correct endpoints    |
| `rate limit exceeded`                        | Quota exhausted                  | Wait for reset or increase limit       |
| `engine shutting down`                       | Server is stopping               | Wait for shutdown, restart server      |
| `huggingface 401 / 403`                      | Gated/private repo or rate limit | Set `KRONK_HF_TOKEN` env var           |
| `model doesn't support embedding`            | Wrong model for endpoint         | Use an embedding model                 |
| `model doesn't support reranking`            | Wrong model for endpoint         | Use a reranking model                  |
| `imc restore failed`                         | RAM→VRAM restore failed          | Auto-recovers; reduce VRAM pressure    |
| `imc extend stale`                           | Concurrent cache modification    | Auto-retries; transient                |

### 16.11 Catalog & Model Pull Issues

**Error: `huggingface 401` / `403` during `kronk model pull`**

The repo is gated/private or the request was throttled.

**Solution:** export a HuggingFace token before pulling. The token must
have read access to any gated repos you intend to use:

```shell
export KRONK_HF_TOKEN=hf_xxx
kronk model pull <provider/model-id>
```

**Problem: `kronk model pull <id>` says "model not found in catalog"**

The id has not been resolved against HuggingFace yet. Use `model resolve`
to look it up and seed the local catalog:

```shell
kronk model resolve https://huggingface.co/<owner>/<repo>
kronk model pull <provider/model-id>
```

**Problem: `catalog.yaml` was hand-edited and Kronk now refuses to start**

The catalog file is YAML keyed by canonical id with strict typing. Run
`kronk catalog list --local` to validate it; remove the broken entry with
`kronk catalog remove <id> --local` and re-add it via `kronk model resolve`.

**Problem: pull failed mid-download, `kronk model list` shows the model
as invalid**

Partial files are left behind under
`~/.kronk/models/<provider>/<family>/`. Remove the model and re-pull:

```shell
kronk model remove <provider/model-id> --local
kronk model pull <provider/model-id>
kronk model index --local
```

### 16.12 MCP Service Issues

**Error: `/mcp` returns 404**

The MCP endpoint is `http://localhost:9000/mcp` (no trailing slash). The
client must use the Streamable HTTP transport — Cline calls it
`streamableHttp`, Kilo Code uses `streamable-http`, and OpenCode/Goose
both spell it `remote`.

**Error: `web_search` reports "missing Brave API key"**

The Brave Search key is unset. Provide it before starting the server:

```shell
# Embedded mode (kronk server start)
export KRONK_MCP_BRAVEAPIKEY=<your-brave-api-key>

# Standalone (make mcp-server)
export MCP_MCP_BRAVEAPIKEY=<your-brave-api-key>
```

**Problem: model can't find the tool ("unknown tool kronk_fuzzy_edit")**

Each MCP-aware client prefixes tool names with the server key. Check
that the prefix matches the key you used in the client config:

| Client   | Server key in config | Tool names exposed                    |
| -------- | -------------------- | ------------------------------------- |
| Cline    | `Kronk`              | `web_search`, `fuzzy_edit`            |
| Kilo     | `Kronk`              | `Kronk_web_search`, `Kronk_fuzzy_edit` |
| OpenCode | `kronk`              | `kronk_web_search`, `kronk_fuzzy_edit` |
| Goose    | (lowercase)          | `kronk_web_search`, `kronk_fuzzy_edit` |

**Error: `fuzzy_edit` returns "old_string not found in file (even with
fuzzy matching)"**

The search snippet is either absent from the file or matches more than
once. Tighten the snippet to a unique block of lines, or break the edit
into a smaller anchor that appears exactly once.

**Problem: embedded MCP server is not starting**

Setting `KRONK_MCP_HOST` to a non-empty value tells `kronk server` to
defer to an external MCP host instead of starting its own. Unset it (or
run the standalone service via `make mcp-server`) if you want the
embedded mode back.

### 16.13 Port Conflicts & Filesystem

**Error: `bind: address already in use`**

Another process is already listening on the port Kronk is trying to
bind. Default ports are `11435` (API), `8090` (debug), and `9000` (MCP).

**Solutions:**

```shell
# Find the offending process
lsof -i :11435

# Or move Kronk to a different port
kronk server start --api-host 0.0.0.0:21435
```

**Error: `permission denied` reading or writing `~/.kronk/`**

Ensure your user owns the kronk base directory. Auth in particular
expects `~/.kronk/keys/` to be `0700`:

```shell
chmod -R u+rwX ~/.kronk
chmod 700 ~/.kronk/keys
```

**Error: `lock file already exists` from BadgerDB
(`~/.kronk/badger/LOCK`)**

Only one Kronk process may hold the rate-limit DB at a time. Confirm no
other server is running, then remove the lock:

```shell
ps aux | grep "kronk server"
rm ~/.kronk/badger/LOCK   # only if no process is using it
```

**Problem: `kronk server start -d` says "already running" but no process
exists**

The PID file is stale. Remove it and start again:

```shell
rm ~/.kronk/kronk.pid
kronk server start -d
```

**Problem: model pull fails with "no space left on device"**

The models directory is full. Free space by removing unused models:

```shell
kronk model list --local
kronk model remove <provider/model-id> --local
```

Models live under `~/.kronk/models/`; check available space with
`df -h ~/.kronk/models`.

### 16.14 Getting Help

**Check server liveness:**

```shell
curl http://localhost:11435/v1/liveness
```

**Check server readiness (model loaded):**

```shell
curl http://localhost:11435/v1/readiness
```

**List loaded models:**

```shell
curl http://localhost:11435/v1/models
```

**Check Prometheus metrics:**

```shell
curl http://localhost:8090/metrics
```

**View goroutine stacks (for hangs):**

```shell
curl http://localhost:8090/debug/pprof/goroutine?debug=2
```

**CPU profile (for slow inference):**

```shell
curl http://localhost:8090/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof cpu.prof
```

**Report issues:**

Include the following when reporting bugs:

- Kronk version (`kronk --version`)
- Operating system and architecture
- GPU type and driver version
- Model name and configuration
- Full error message and stack trace
- Steps to reproduce

---

_Next: [Chapter 17: Developer Guide](chapter-17-developer-guide.md)_
