# Chapter 16: Troubleshooting

## Table of Contents

- [16.1 Library Issues](#161-library-issues)
- [16.2 Model Loading Failures](#162-model-loading-failures)
- [16.3 Memory Errors](#163-memory-errors)
- [16.4 Request Timeouts](#164-request-timeouts)
- [16.5 Authentication Errors](#165-authentication-errors)
- [16.6 Streaming Issues](#166-streaming-issues)
- [16.7 Performance Issues](#167-performance-issues)
- [16.8 Viewing Logs](#168-viewing-logs)
- [16.9 Common Error Messages](#169-common-error-messages)
- [16.10 Getting Help](#1610-getting-help)

---



This chapter covers common issues, their causes, and solutions.

### 16.1 Library Issues

**Error: "unable to load library"**

The llama.cpp libraries are missing or incompatible.

**Solution:**

```shell
kronk libs --local
```

Or download via the BUI Libraries page.

**Error: "unknown device"**

The specified GPU device is not available.

**Causes:**

- Wrong `--device` flag (e.g., `cuda` on a Mac)
- GPU drivers not installed
- Library mismatch (CPU library with GPU device setting)

**Solution:**

Check your hardware and install matching libraries:

```shell
# For Mac with Apple Silicon
KRONK_PROCESSOR=metal kronk libs --local

# For NVIDIA GPU
KRONK_PROCESSOR=cuda kronk libs --local

# For CPU only
KRONK_PROCESSOR=cpu kronk libs --local

# For AMD GPU (ROCm)
KRONK_PROCESSOR=rocm kronk libs --local
```

### 16.2 Model Loading Failures

**Error: "unable to load model"**

The model file is missing, corrupted, or incompatible.

**Check model exists:**

```shell
ls ~/.kronk/models/
```

**Re-download the model:**

```shell
kronk catalog pull <model-name> --local
```

**Verify model integrity:**

By default, Kronk skips integrity checks. To force verification:

```shell
kronk server start --ignore-integrity-check=false
```

**Error: "failed to retrieve model template"**

The model's chat template is missing.

**Solution:**

Ensure templates are downloaded:

```shell
kronk catalog pull-templates --local
```

### 16.3 Memory Errors

**Error: "unable to init context" or "unable to get memory"**

Insufficient memory for the model configuration.

**Causes:**

- Context window too large
- Too many batch slots
- Model too large for available RAM/VRAM

**Solutions:**

Reduce context window:

```yaml
models:
  Qwen3-8B-Q8_0:
    context_window: 8192 # Reduce from 32768
```

Reduce batch parallelism:

```yaml
models:
  Qwen3-8B-Q8_0:
    n_seq_max: 1 # Single request at a time
```

Use quantized KV cache:

```yaml
models:
  Qwen3-8B-Q8_0:
    cache-type-k: q8_0 # Saves ~50% KV cache memory
    cache-type-v: q8_0
```

**Error: "context window is full"**

The request plus context exceeds the configured context window.

**Solutions:**

- Reduce input size (fewer messages or shorter prompts)
- Increase `context_window` in model config
- Enable YaRN for extended context (see Chapter 6)

### 16.4 Request Timeouts

**Error: "context deadline exceeded"**

The request took longer than the configured timeout.

**Causes:**

- Model too slow for the request size
- Large prefill with many tokens
- Server under heavy load

**Solutions:**

Increase HTTP timeouts:

```shell
kronk server start \
  --read-timeout 5m \
  --write-timeout 30m
```

Or via environment variables:

```shell
export KRONK_READ_TIMEOUT=5m
export KRONK_WRITE_TIMEOUT=30m
```

### 16.5 Authentication Errors

**Error: "unauthorized: no authorization header"**

Authentication is enabled but no token was provided.

**Solution:**

Include the Authorization header:

```shell
curl http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $(cat ~/.kronk/keys/master.jwt)" \
  -H "Content-Type: application/json" \
  -d '{...}'
```

**Error: "invalid token"**

The token is malformed, expired, or signed with an unknown key.

**Causes:**

- Token has expired (check `--duration` when created)
- Signing key was deleted
- Token is corrupted

**Solution:**

Create a new token:

```shell
export KRONK_TOKEN=$(cat ~/.kronk/keys/master.jwt)
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings
```

**Error: "endpoint not authorized"**

The token doesn't include the requested endpoint.

**Solution:**

Create a new token with the required endpoints:

```shell
kronk security token create \
  --duration 720h \
  --endpoints chat-completions,embeddings,rerank,responses,messages
```

**Error: "rate limit exceeded"**

The token has exceeded its rate limit.

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

- Client disconnected
- Request timeout
- Model generated stop token

**Check server logs:**

```shell
# Look for errors in server output
kronk server start  # Run in foreground to see logs
```

**Problem: SSE events not parsing correctly**

Ensure your client handles Server-Sent Events format:

```
data: {"id":"...","choices":[...]}\n\n
```

Each event is prefixed with `data: ` and ends with two newlines.

### 16.7 Performance Issues

**Problem: Slow time to first token (TTFT)**

**Causes:**

- Large system prompt not cached
- No message caching enabled
- Cold model load

**Solutions:**

Enable system prompt caching:

```yaml
models:
  Qwen3-8B-Q8_0:
    system_prompt_cache: true
```

Or enable incremental message cache for agents:

```yaml
models:
  Qwen3-8B-Q8_0:
    incremental_cache: true
```

**Problem: Slow token generation (tokens/second)**

**Causes:**

- CPU inference instead of GPU
- Insufficient GPU layers
- Large model for available hardware

**Solutions:**

Check GPU is being used:

```shell
# On macOS, check Metal usage
sudo powermetrics --samplers gpu_power

# On Linux with NVIDIA
nvidia-smi
```

Increase GPU layers:

```yaml
models:
  Qwen3-8B-Q8_0:
    gpu_layers: 99 # Offload all layers to GPU
```

### 16.8 Viewing Logs

**Run server in foreground:**

```shell
kronk server start
```

All logs print to stdout with structured JSON format.

**Enable verbose logging:**

```shell
kronk server start --insecure-logging
```

This logs full message content (never use in production).

**Enable llama.cpp logging:**

```shell
kronk server start --llama-log 1
```

Shows low-level inference engine messages.

**Disable llama.cpp logging:**

```shell
kronk server start --llama-log 0
```

### 16.9 Common Error Messages

| Error                  | Cause                  | Solution               |
| ---------------------- | ---------------------- | ---------------------- |
| `Init() not called`    | Missing initialization | Call `kronk.Init()`    |
| `unknown device`       | Invalid GPU setting    | Check `--device` flag  |
| `context deadline`     | Request timeout        | Increase timeouts      |
| `unable to load model` | Missing/corrupt model  | Re-download model      |
| `no authorization`     | Missing token          | Add Bearer token       |
| `rate limit exceeded`  | Quota exhausted        | Wait or increase limit |
| `context window full`  | Input too large        | Reduce input size      |
| `NBatch overflow`      | Batch too large        | Reduce `n_batch`       |

### 16.10 Getting Help

**Check server status:**

```shell
curl http://localhost:8080/v1/liveness
```

**List loaded models:**

```shell
curl http://localhost:8080/v1/models
```

**Check metrics:**

```shell
curl http://localhost:8090/metrics
```

**View goroutine stacks (for hangs):**

```shell
curl http://localhost:8090/debug/pprof/goroutine?debug=2
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

_Next: [Chapter 17: Developer Guide](#chapter-17-developer-guide)_
