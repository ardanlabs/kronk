# Chapter 15: Observability

## Table of Contents

- [15.1 Debug and Health Endpoints](#151-debug-and-health-endpoints)
  - [Debug Server](#debug-server)
  - [Health Checks](#health-checks)
- [15.2 Prometheus Metrics](#152-prometheus-metrics)
  - [Metric Groups](#metric-groups)
  - [PromQL Examples](#promql-examples)
- [15.3 Bundled Observability Stack](#153-bundled-observability-stack)
- [15.4 OpenTelemetry Tracing](#154-opentelemetry-tracing)
- [15.5 Profiling and Runtime Visualization](#155-profiling-and-runtime-visualization)
- [15.6 Logging](#156-logging)

---

Kronk exposes health checks on the Web API and runs a separate debug server
for metrics, profiling, and runtime visualization. It can also export traces
to an OTLP gRPC collector such as Grafana Tempo. Structured logs are written
to standard output.

### 15.1 Debug and Health Endpoints

#### Debug Server

The main API binds to `0.0.0.0:11435` by default. Observability endpoints use
a separate server at `0.0.0.0:11445`:

| Path | Purpose |
| ---- | ------- |
| `/metrics` | Prometheus metrics |
| `/debug/pprof/` | Go profile index |
| `/debug/pprof/profile` | CPU profile |
| `/debug/pprof/heap` | Heap profile |
| `/debug/pprof/goroutine` | Goroutine profile |
| `/debug/pprof/trace` | Go execution trace |
| `/debug/statsviz` | Live Go runtime charts |

> **Security:** The debug server has no authentication and its default address
> listens on every interface. Profiles and runtime data can reveal sensitive
> operational details. Bind it to loopback or restrict port `11445` at the
> network boundary unless remote scraping is required.

To bind it to loopback:

```shell
kronk server start --debug-host localhost:11445
```

The equivalent environment variable is
`KRONK_WEB_DEBUG_HOST=localhost:11445`.

#### Health Checks

The unauthenticated health routes are served from the main API port:

```shell
curl http://localhost:11435/v1/liveness
curl http://localhost:11435/v1/readiness
```

Liveness returns JSON containing `status`, `build`, `host`, and `GOMAXPROCS`.
Readiness currently returns an empty `200 OK` when the HTTP service is
running. It does not validate model files, inference libraries, devices,
available memory, or a loaded model.

### 15.2 Prometheus Metrics

Fetch the current metric inventory and its `HELP` descriptions from:

```shell
curl http://localhost:11445/metrics
```

The endpoint includes Go `go_*` metrics, process `process_*` metrics, and
Kronk metrics. The following groups are useful starting points; `/metrics` is
the authoritative list.

#### Metric Groups

| Area | Representative metrics |
| ---- | ---------------------- |
| HTTP | `requests`, `errors`, `panics`, `goroutines` |
| Model loading | `model_load_seconds`, `model_load_proj_seconds` |
| Inference latency | `model_prompt_creation_seconds`, `model_prefill_seconds`, `model_prefill_ttft_seconds`, `model_request_ttft_seconds` |
| Requests | `chat_requests_total`, `chat_errors_total`, `chat_request_duration_seconds`, `chat_queue_wait_seconds` |
| Tokens | `usage_tokens_total`, `usage_tokens_per_second` |
| Model memory | `vram_total_bytes`, `vram_slot_memory_bytes` |
| Pool | `pool_acquire_total`, `pool_evictions_total`, `pool_items_in_pool`, `pool_max_items_in_pool`, `pool_active_streams`, `pool_inflight_loads` |
| Resource manager | `resman_ram_used_bytes`, `resman_device_used_bytes`, `resman_reservation_bytes`, `resman_reserve_rejections_total` |
| IMC | `imc_snapshot_skipped_total`, `imc_pure_hit_stale_session_total` |

Most model and request metrics have a `model_id` label. Histograms expose
`_bucket`, `_sum`, and `_count` series. Counters such as
`usage_tokens_total` should normally be queried with `rate()` or `increase()`.

For an external Prometheus process on the same host:

```yaml
scrape_configs:
  - job_name: "kronk"
    static_configs:
      - targets: ["localhost:11445"]
    scrape_interval: 15s
```

When Prometheus runs in Docker while Kronk runs on the host, use
`host.docker.internal:11445`, as the repository's configuration does.

#### PromQL Examples

Average end-to-end time to first token by model:

```promql
rate(model_request_ttft_seconds_sum[5m])
  / rate(model_request_ttft_seconds_count[5m])
```

P99 time to first token by model:

```promql
histogram_quantile(0.99,
  sum by (le, model_id) (rate(model_request_ttft_seconds_bucket[5m])))
```

Token throughput by model and kind:

```promql
sum by (model_id, kind) (rate(usage_tokens_total[5m]))
```

### 15.3 Bundled Observability Stack

The repository includes a Docker Compose stack containing Grafana, Prometheus,
Tempo, Loki, and Promtail. It provisions the data sources and a Kronk dashboard
without manual Grafana setup.

Download the pinned images once, start the stack, and open Grafana:

```shell
make install-docker
make grafana-up
make grafana-browse
```

Grafana is served at `http://localhost:3100/`. Prometheus scrapes the host's
Kronk debug server, and Tempo accepts OTLP gRPC traces on port `4317`.

Stop the stack with:

```shell
make grafana-down
```

### 15.4 OpenTelemetry Tracing

Kronk exports OpenTelemetry traces over unencrypted OTLP gRPC. The defaults are:

| Setting | Flag | Environment variable | Default |
| ------- | ---- | -------------------- | ------- |
| Collector | `--tempo-host` | `KRONK_TEMPO_HOST` | `localhost:4317` |
| Service | `--tempo-service-name` | `KRONK_TEMPO_SERVICE_NAME` | `kronk` |
| Sampling | `--tempo-probability` | `KRONK_TEMPO_PROBABILITY` | `0.25` |

No collector is required for startup. Until one is reachable, spans are
non-recording and Kronk probes the configured address every 60 seconds. A
later successful connection activates tracing without restarting the server.

For a remote collector or a different sampling rate:

```shell
kronk server start \
  --tempo-host otel-collector.example.com:4317 \
  --tempo-probability 0.05
```

The HTTP layer accepts W3C Trace Context headers and creates request and route
spans. Inference adds spans for request preparation, cache processing,
queueing, prefill, token generation, and selected model operations. Liveness
and readiness routes are excluded from sampling.

Set the probability to `1.0` when every trace is needed for focused debugging,
or `0.0` to disable sampling. Choose a lower nonzero value for sustained
production traffic based on its volume and storage budget.

### 15.5 Profiling and Runtime Visualization

Use Go's pprof tooling against the debug server. For example:

```shell
go tool pprof http://localhost:11445/debug/pprof/profile?seconds=30
go tool pprof http://localhost:11445/debug/pprof/heap
curl http://localhost:11445/debug/pprof/goroutine?debug=2
```

Start pprof's interactive web interface with:

```shell
go tool pprof -http=localhost:8081 \
  http://localhost:11445/debug/pprof/profile?seconds=30
```

Statsviz displays live heap, allocation, goroutine, garbage collection, and
scheduler charts at:

```text
http://localhost:11445/debug/statsviz
```

The same unauthenticated-access warning as the rest of the debug server applies
to pprof and Statsviz.

### 15.6 Logging

Kronk logs structured JSON to stdout by default.

Log records include the service name, source location, severity, and a trace
ID. A generated request ID is used when a sampled OpenTelemetry trace ID is not
available, so logs remain correlatable even without a collector.

Sensitive prompts, responses, and detailed model configuration are omitted by
default. They can be included temporarily for local debugging:

```shell
kronk server start --insecure-logging
```

The environment equivalent is `KRONK_INSECURE_LOGGING=true`. Do not enable
this on production systems or when logs are sent to a shared collector.

---

_Next: [Chapter 16: MCP Service](chapter-16-mcp-service.md)_
