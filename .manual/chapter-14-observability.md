# Chapter 14: Observability

## Table of Contents

- [14.1 Debug Server](#141-debug-server)
- [14.2 Debug Endpoints](#142-debug-endpoints)
- [14.3 Health Check Endpoints](#143-health-check-endpoints)
- [14.4 Prometheus Metrics](#144-prometheus-metrics)
- [14.5 Prometheus Integration](#145-prometheus-integration)
- [14.6 Distributed Tracing with Tempo](#146-distributed-tracing-with-tempo)
- [14.7 Tracing Architecture](#147-tracing-architecture)
- [14.8 Tempo Setup with Docker](#148-tempo-setup-with-docker)
- [14.9 pprof Profiling](#149-pprof-profiling)
- [14.10 Statsviz Real-Time Monitoring](#1410-statsviz-real-time-monitoring)
- [14.11 Logging](#1411-logging)
- [14.12 Configuration Reference](#1412-configuration-reference)

---



Kronk provides comprehensive observability through distributed tracing,
Prometheus metrics, pprof profiling, and real-time visualizations.

### 14.1 Debug Server

Kronk runs a separate debug server for observability endpoints, isolated
from the main API for security.

**Default Ports:**

- Main API: `localhost:8080`
- Debug server: `localhost:8090`

**Configure Debug Host:**

```shell
kronk server start --debug-host localhost:9090
```

Or via environment variable:

```shell
export KRONK_DEBUG_HOST=localhost:9090
kronk server start
```

### 14.2 Debug Endpoints

The debug server exposes these endpoints:

**Prometheus Metrics:**

```
http://localhost:8090/metrics
```

**pprof Profiling:**

- `http://localhost:8090/debug/pprof/` - Index page
- `http://localhost:8090/debug/pprof/profile` - CPU profile
- `http://localhost:8090/debug/pprof/heap` - Heap profile
- `http://localhost:8090/debug/pprof/goroutine` - Goroutine stacks
- `http://localhost:8090/debug/pprof/trace` - Execution trace

**Statsviz (Real-time Visualizations):**

```
http://localhost:8090/debug/statsviz
```

Provides live charts for memory, goroutines, GC, and more.

### 14.3 Health Check Endpoints

Available on the main API port (no authentication required):

**Liveness Check:**

```shell
curl http://localhost:8080/v1/liveness
```

Response:

```json
{
  "status": "up",
  "build": "v1.0.0",
  "host": "hostname",
  "GOMAXPROCS": 8
}
```

**Readiness Check:**

```shell
curl http://localhost:8080/v1/readiness
```

Returns 200 OK when the server is ready to accept requests.

### 14.4 Prometheus Metrics

Kronk exposes detailed inference metrics in Prometheus format.

**Fetch Metrics:**

```shell
curl http://localhost:8090/metrics
```

**Available Metrics:**

System metrics:

- `goroutines` - Current goroutine count
- `requests` - Total request count
- `errors` - Total error count
- `panics` - Total panic count

Model loading (in seconds):

- `model_load_avg`, `model_load_min`, `model_load_max`
- `model_load_proj_avg`, `model_load_proj_min`, `model_load_proj_max`

Inference timing (in seconds):

- `model_prompt_creation_avg`, `_min`, `_max`
- `model_prefill_avg`, `_min`, `_max`
- `model_ttft_avg`, `_min`, `_max` (time to first token)

Token usage:

- `usage_prompt_tokens_avg`, `_min`, `_max`
- `usage_reasoning_tokens_avg`, `_min`, `_max`
- `usage_completion_tokens_avg`, `_min`, `_max`
- `usage_output_tokens_avg`, `_min`, `_max`
- `usage_total_tokens_avg`, `_min`, `_max`
- `usage_tokens_per_second_avg`, `_min`, `_max`

### 14.5 Prometheus Integration

**Example Prometheus Configuration:**

```yaml
# prometheus.yml
scrape_configs:
  - job_name: "kronk"
    static_configs:
      - targets: ["localhost:8090"]
    scrape_interval: 15s
```

**Grafana Dashboard Query Examples:**

Time to first token:

```promql
model_ttft_avg
```

Tokens per second throughput:

```promql
usage_tokens_per_second_avg
```

Request rate:

```promql
rate(requests[5m])
```

Error rate:

```promql
rate(errors[5m]) / rate(requests[5m])
```

### 14.6 Distributed Tracing with Tempo

Kronk supports OpenTelemetry tracing with Grafana Tempo integration.

**Enable Tracing:**

```shell
kronk server start \
  --tempo-host localhost:4317 \
  --tempo-service-name kronk \
  --tempo-probability 0.25
```

Or via environment variables:

```shell
export KRONK_TEMPO_HOST=localhost:4317
export KRONK_TEMPO_SERVICE_NAME=kronk
export KRONK_TEMPO_PROBABILITY=0.25
kronk server start
```

**Configuration Options:**

- `--tempo-host` - Tempo collector address (OTLP gRPC endpoint)
- `--tempo-service-name` - Service name in traces (default: `kronk`)
- `--tempo-probability` - Sampling probability 0.0-1.0 (default: `0.25`)

**Sampling Probability:**

- `1.0` - Trace every request (development only)
- `0.25` - Trace 25% of requests (recommended for production)
- `0.05` - Trace 5% of requests (high-traffic production)

**Excluded Routes:**

Health check endpoints are automatically excluded from tracing:

- `/v1/liveness`
- `/v1/readiness`

### 14.7 Tracing Architecture

**Request Flow with Tracing:**

```
Client Request
      │
      ▼
┌─────────────────────────────┐
│  Kronk Server               │
│  ┌───────────────────────┐  │
│  │ Inject Trace Context  │  │
│  │ (trace_id, span_id)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Handler Span          │  │
│  │ (chat, embed, etc.)   │  │
│  └───────────┬───────────┘  │
│              ▼              │
│  ┌───────────────────────┐  │
│  │ Inference Span        │  │
│  │ (model operations)    │  │
│  └───────────────────────┘  │
└─────────────────────────────┘
      │
      ▼
   Tempo Collector (OTLP gRPC)
      │
      ▼
   Grafana (Visualization)
```

**What Gets Traced:**

- HTTP request handling
- Model acquisition from pool
- Prefill and generation phases
- Token streaming

### 14.8 Tempo Setup with Docker

**Run Tempo Locally:**

```shell
docker run -d --name tempo \
  -p 3200:3200 \
  -p 4317:4317 \
  grafana/tempo:latest \
  -config.file=/etc/tempo/tempo.yaml
```

**Run Grafana:**

```shell
docker run -d --name grafana \
  -p 3000:3000 \
  grafana/grafana:latest
```

**Configure Grafana:**

1. Open http://localhost:3000 (admin/admin)
2. Add data source → Tempo
3. Set URL: `http://tempo:3200`
4. Save and explore traces

### 14.9 pprof Profiling

Use Go's pprof tools for performance analysis.

**Capture CPU Profile (30 seconds):**

```shell
go tool pprof http://localhost:8090/debug/pprof/profile?seconds=30
```

**Capture Heap Profile:**

```shell
go tool pprof http://localhost:8090/debug/pprof/heap
```

**View Goroutine Stacks:**

```shell
curl http://localhost:8090/debug/pprof/goroutine?debug=2
```

**Generate Flame Graph:**

```shell
go tool pprof -http=:8081 \
  http://localhost:8090/debug/pprof/profile?seconds=30
```

Opens interactive web UI with flame graph visualization.

### 14.10 Statsviz Real-Time Monitoring

Statsviz provides live runtime visualizations in your browser.

**Access Statsviz:**

```
http://localhost:8090/debug/statsviz
```

**Available Charts:**

- Heap size and allocations
- Goroutine count
- GC pause times
- CPU scheduler latency
- Memory by size class

Useful for real-time monitoring during load testing or debugging
memory issues.

### 14.11 Logging

Kronk logs structured JSON to stdout by default.

**Log Levels:**

Logs include context like trace IDs, request details, and timing.

**Insecure Logging:**

For debugging, enable verbose logging that includes message content:

```shell
kronk server start --insecure-logging
```

**Warning:** Insecure logging exposes user prompts and model responses.
Never enable in production.

**Environment Variable:**

```shell
export KRONK_INSECURE_LOGGING=true
```

### 14.12 Configuration Reference

**Debug Server:**

- `--debug-host` - Debug server address (env: `KRONK_DEBUG_HOST`,
  default: `localhost:8090`)

**Tracing:**

- `--tempo-host` - Tempo collector address (env: `KRONK_TEMPO_HOST`,
  default: `localhost:4317`)
- `--tempo-service-name` - Service name (env: `KRONK_TEMPO_SERVICE_NAME`,
  default: `kronk`)
- `--tempo-probability` - Sampling rate 0.0-1.0
  (env: `KRONK_TEMPO_PROBABILITY`, default: `0.25`)

**Logging:**

- `--insecure-logging` - Log message content
  (env: `KRONK_INSECURE_LOGGING`, default: `false`)
- `--llama-log` - llama.cpp log level, 0=off, 1=on
  (env: `KRONK_LLAMA_LOG`, default: `1`)

---

_Next: [Chapter 15: MCP Service](#chapter-15-mcp-service)_
