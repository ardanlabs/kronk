/*
Benchmarks for the three inference caching modes: Non-Caching, SPC, and IMC.

Model: Qwen3-8B-Q8_0

Modes Tested:
  - NonCaching: Baseline with no caching. Full prefill on every request.
  - SPC (System Prompt Cache): Caches the system prompt KV state. The system
    prompt is decoded once and its KV state is externalized to a byte buffer
    in RAM. Subsequent requests restore from the buffer, skipping redundant
    prefill of the system prompt.
  - IMC (Incremental Message Cache): Caches all messages except the last.
    The cache extends incrementally on each turn. Ideal for agentic workflows
    where conversations grow monotonically.

Conversation Structure (~30k of 32k tokens):
  - System prompt (~10k tokens): Large system prompt simulating a real-world
    agentic workflow (similar to Cline, Cursor, etc.) with detailed technical
    competencies, code examples, API references, and project context. Must be
    large enough that SPC's KV state restore is faster than re-prefilling.
    At ~800 tokens the save/restore overhead exceeds re-prefill cost, so we
    target ~10k tokens where SPC shows clear benefit.
  - ~15+ conversation turns (~20k tokens): 6 unique technical Q&A pairs
    (GC tuning, PostgreSQL query optimization, Kafka partitioning, Redis
    caching, observability) cycled ~3 times with turn-number suffixes to
    avoid degenerate tokenization. Exercises IMC caching.
  - max_tokens=128: Small output keeps the benchmark focused on prefill and
    caching performance rather than generation.
  - temperature=0.0: Deterministic output for consistency across runs.

Metrics Reported Per Iteration:
  - ttft-ms     Time To First Token in milliseconds.
  - tok/s       Tokens per second (decode throughput).
  - total-ms    Wall-clock end-to-end request time in milliseconds.
  - prompt-tok  Prompt token count (consistency check across runs).
  - output-tok  Output token count (consistency check across runs).
  - B/op        Bytes allocated per operation (via ReportAllocs).
  - allocs/op   Number of allocations per operation (via ReportAllocs).

Running:

	go test -bench=. -benchtime=3x -timeout=60m ./sdk/kronk/model/
	go test -bench=BenchmarkSPC -benchtime=5x -timeout=60m ./sdk/kronk/model/
*/
package model_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// =============================================================================
// Test setup - model path resolved once

var benchModelPath models.Path

func TestMain(m *testing.M) {
	mdls, err := models.New()
	if err != nil {
		fmt.Printf("bench: unable to create models system: %v\n", err)
		os.Exit(1)
	}

	benchModelPath = mdls.MustFullPath("Qwen3-8B-Q8_0")

	ctlg, err := catalog.New()
	if err != nil {
		fmt.Printf("bench: unable to create catalog: %v\n", err)
		os.Exit(1)
	}

	if err := ctlg.Download(context.Background()); err != nil {
		fmt.Printf("bench: unable to download catalog: %v\n", err)
		os.Exit(1)
	}

	if err := kronk.Init(); err != nil {
		fmt.Printf("bench: unable to init kronk: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// =============================================================================
// Config builders for the three modes
//
// Context window: 32768
// Target prompt fill: ~30k tokens (~105k chars), leaving room for 128 output.

const benchContextWindow = 32768

func cfgNonCaching() model.Config {
	return model.Config{
		ModelFiles:    benchModelPath.ModelFiles,
		ContextWindow: benchContextWindow,
		NBatch:        2048,
		NUBatch:       512,
		CacheTypeK:    model.GGMLTypeQ8_0,
		CacheTypeV:    model.GGMLTypeQ8_0,
		NSeqMax:       1,
	}
}

func cfgSPC() model.Config {
	return model.Config{
		ModelFiles:        benchModelPath.ModelFiles,
		ContextWindow:     benchContextWindow,
		NBatch:            2048,
		NUBatch:           512,
		CacheTypeK:        model.GGMLTypeQ8_0,
		CacheTypeV:        model.GGMLTypeQ8_0,
		NSeqMax:           1,
		SystemPromptCache: true,
	}
}

func cfgIMC() model.Config {
	return model.Config{
		ModelFiles:       benchModelPath.ModelFiles,
		ContextWindow:    benchContextWindow,
		NBatch:           2048,
		NUBatch:          512,
		CacheTypeK:       model.GGMLTypeQ8_0,
		CacheTypeV:       model.GGMLTypeQ8_0,
		NSeqMax:          1,
		IncrementalCache: true,
	}
}

// =============================================================================
// Benchmark conversation generator
//
// Builds a multi-turn conversation that fills ~30k tokens of a 32k context
// window. Uses a large system prompt (~10k tokens) plus many conversation
// turns with substantial assistant responses (~20k tokens).
//
// Qwen3-8B tokenizer averages ~3.5 chars per token for English prose.
// Target: ~105k chars total → ~30k tokens, leaving ~2k tokens for output.

// targetPromptChars is the approximate character count we aim for across all
// messages. At ~3.5 chars/token this yields ~30k tokens.
const targetPromptChars = 105_000

func benchDoc() model.D {
	messages := buildConversation()

	return model.D{
		"messages":    messages,
		"max_tokens":  128,
		"temperature": 0.0,
	}
}

func buildConversation() []model.D {
	var messages []model.D
	totalChars := 0

	// System prompt: detailed instructions (~7k chars ≈ 2k tokens).
	sys := buildSystemPrompt()
	messages = append(messages, model.D{"role": "system", "content": sys})
	totalChars += len(sys)

	// Conversation turns: cycle through Q&A pairs until we approach the target.
	// Reserve the last user message for the generation prompt.
	turns := conversationTurns()
	turnIdx := 0

	for totalChars < targetPromptChars {
		turn := turns[turnIdx%len(turns)]
		turnIdx++

		// Vary the content slightly per cycle to avoid degenerate tokenization.
		suffix := fmt.Sprintf(" [Turn %d]", turnIdx)

		userMsg := turn.question + suffix
		messages = append(messages, model.D{"role": "user", "content": userMsg})
		totalChars += len(userMsg)

		if totalChars >= targetPromptChars {
			break
		}

		assistMsg := turn.answer + suffix
		messages = append(messages, model.D{"role": "assistant", "content": assistMsg})
		totalChars += len(assistMsg)
	}

	// Ensure the final message is a user message to trigger generation.
	last := messages[len(messages)-1]
	if role, _ := last["role"].(string); role != "user" {
		messages = append(messages, model.D{
			"role":    "user",
			"content": "Summarize the key points from our conversation in two sentences.",
		})
	}

	return messages
}

// buildSystemPrompt generates a ~47k character system prompt (~10k tokens).
// This simulates real-world agentic system prompts (Cline, Cursor, etc.) that
// contain detailed instructions, API references, code examples, and project
// context. At ~800 tokens, SPC overhead exceeds re-prefill cost. At ~10k
// tokens, SPC shows clear benefit.
func buildSystemPrompt() string {
	return `You are a senior software architect and technical advisor specializing in distributed systems, cloud-native applications, and high-performance computing. Your role is to provide thorough, well-reasoned technical guidance that considers trade-offs, scalability implications, and operational concerns.

## 1. Systems Design & Architecture

You understand microservices, monoliths, event-driven architectures, CQRS, event sourcing, and domain-driven design. You can evaluate trade-offs between consistency and availability, and you understand the implications of the CAP theorem in practical distributed systems. You are well-versed in service mesh architectures, API gateway patterns, and circuit breaker implementations.

When designing distributed systems, you consider the following architectural patterns and their trade-offs in detail:

### Microservices vs Monolith
You understand that microservices introduce network latency, distributed transaction complexity, and operational overhead. You recommend starting with a modular monolith and extracting services only when there is a clear scaling or deployment boundary that justifies the complexity. You can identify the right service boundaries using domain-driven design bounded contexts and evaluate whether a service split improves or worsens the overall system reliability. Key considerations include: data ownership boundaries, team autonomy requirements, independent deployment needs, and the operational cost of running additional infrastructure. You know that premature decomposition is one of the most expensive architectural mistakes, often requiring expensive re-merging of services that were split too early.

### Event-Driven Architecture
You understand the difference between event notification, event-carried state transfer, and event sourcing. You know when to use choreography vs orchestration for saga patterns. You can design idempotent event handlers and implement exactly-once semantics using deduplication tables or idempotency keys. You understand the challenges of event ordering, schema evolution, and event versioning in long-lived systems. For event schema evolution, you follow the compatibility rules: new fields must be optional, existing fields must not be removed or renamed, and consumers must ignore unknown fields. You use schema registries (Confluent Schema Registry, AWS Glue) to enforce compatibility checks at build time rather than discovering incompatibilities in production.

### CQRS and Event Sourcing
You understand that CQRS separates read and write models, allowing each to be optimized independently. Event sourcing provides a complete audit trail and enables temporal queries, but introduces complexity in event schema evolution, snapshot management, and projection rebuilds. You can evaluate whether the benefits justify the complexity for a given use case and design appropriate snapshotting strategies to keep replay times bounded. You know that event sourcing is most valuable in domains with complex business rules, audit requirements, and the need to reconstruct historical state. For simpler CRUD domains, you recommend traditional state-based persistence.

### Service Mesh and API Gateway
You understand how service meshes like Istio and Linkerd handle mTLS, traffic management, observability, and retry policies at the infrastructure layer. You can evaluate the operational overhead of running a service mesh — including the CPU and memory cost of sidecar proxies, the complexity of debugging through proxied connections, and the learning curve for the operations team. For API gateways, you understand rate limiting algorithms (token bucket, sliding window, leaky bucket), authentication offloading (JWT validation, OAuth token introspection), request transformation, and canary routing patterns. You can design gateway configurations that handle graceful degradation during upstream failures.

### Distributed Consensus
You understand Raft and Paxos consensus protocols at an implementation level. You know how leader election works through term-based voting, how log replication ensures consistency through append-only commit logs, and how membership changes are handled safely through joint consensus. You can evaluate the performance implications of consensus: writes require a majority quorum (3 nodes = 2 must acknowledge), cross-datacenter consensus adds 50-200ms of latency per round trip, and leader failures trigger election timeouts of 150-300ms typically. You understand the difference between strong consistency (linearizable reads from leader), bounded staleness (follower reads within a time window), and eventual consistency (any replica can serve reads).

## 2. Performance Engineering

You can analyze bottlenecks in CPU-bound and I/O-bound workloads, optimize memory allocation patterns, reduce GC pressure in managed runtimes, and design lock-free data structures. You understand cache hierarchies (L1/L2/L3), NUMA topology, and how memory access patterns affect throughput.

### Go Runtime Internals
You understand the Go scheduler's GMP model: G (goroutines) are multiplexed onto M (OS threads), which are bound to P (logical processors). The work-stealing scheduler distributes runnable goroutines across processors for load balancing. You know how to tune GOMAXPROCS for containerized environments (use automaxprocs to detect cgroup CPU limits) and understand the interaction between Go's runtime and Linux cgroups v2 CPU bandwidth controllers. You can diagnose goroutine leaks using runtime.NumGoroutine() and pprof goroutine profiles, and you understand the cost model: goroutine creation is ~2-4KB stack allocation, channel send/receive is ~50-100ns uncontended, and mutex Lock/Unlock is ~20-30ns uncontended but degrades rapidly under contention.

Example of detecting goroutine leaks in tests:
` + "```go" + `
func TestNoGoroutineLeak(t *testing.T) {
    before := runtime.NumGoroutine()
    
    // Run the code under test
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    result, err := processEvents(ctx, events)
    require.NoError(t, err)
    
    // Allow goroutines to settle
    time.Sleep(100 * time.Millisecond)
    
    after := runtime.NumGoroutine()
    if after > before+2 {
        buf := make([]byte, 1<<20)
        n := runtime.Stack(buf, true)
        t.Fatalf("goroutine leak: before=%d after=%d\n%s", before, after, buf[:n])
    }
}
` + "```" + `

### Garbage Collection Tuning
You understand Go's concurrent tri-color mark-and-sweep collector and the two primary tuning knobs: GOGC (controls heap growth ratio, default 100 meaning GC triggers when heap doubles) and GOMEMLIMIT (hard memory ceiling that prevents OOM by triggering more aggressive GC). You know how to reduce GC pressure through: object pooling with sync.Pool (recycle frequently allocated objects), escape analysis optimization (keep objects on the stack by avoiding pointer indirection), pre-allocated slices and maps (avoid repeated growth allocations), and value types over pointer types (reduce heap object count).

You can read and interpret GC traces from GODEBUG=gctrace=1 output:
` + "```" + `
gc 1 @0.012s 2%: 0.024+1.45+0.018 ms clock, 0.19+0.40/1.15/0+0.14 ms cpu, 4->5->1 MB, 5 MB goal, 8 P
` + "```" + `

Where: 2% is total CPU spent in GC, 0.024ms is STW mark setup, 1.45ms is concurrent marking, 0.018ms is STW mark termination, 4->5->1 MB is heap before/after/live, 5 MB is the target heap size. You know that STW pauses are typically under 1ms in Go 1.19+ and that most GC latency comes from assist marking (goroutines helping mark when allocating too fast).

Example of effective sync.Pool usage:
` + "```go" + `
var bufPool = sync.Pool{
    New: func() any {
        return &bytes.Buffer{}
    },
}

func processRequest(data []byte) ([]byte, error) {
    buf := bufPool.Get().(*bytes.Buffer)
    defer func() {
        buf.Reset()
        bufPool.Put(buf)
    }()
    
    // Use buf for processing
    buf.Write(data)
    return transform(buf.Bytes())
}
` + "```" + `

### Memory Access Patterns
You understand that modern CPUs are fundamentally limited by memory bandwidth and cache miss rates rather than raw compute throughput. Sequential access patterns are 10-100x faster than random access due to hardware prefetching. Cache line sizes are 64 bytes on x86-64 and 128 bytes on Apple Silicon M-series chips. You design data structures for cache efficiency by keeping frequently accessed fields together, avoiding false sharing in concurrent code by padding shared variables to cache line boundaries, and preferring struct-of-arrays layout when processing large datasets field by field.

False sharing prevention example:
` + "```go" + `
type PerCPUCounter struct {
    counters []paddedCounter
}

type paddedCounter struct {
    value atomic.Int64
    _     [120]byte // Pad to 128 bytes (Apple Silicon cache line)
}

func NewPerCPUCounter(n int) *PerCPUCounter {
    return &PerCPUCounter{
        counters: make([]paddedCounter, n),
    }
}
` + "```" + `

### Lock-Free Programming
You understand atomic operations (CAS, load-acquire, store-release), memory ordering guarantees provided by sync/atomic in Go, and the ABA problem. You can implement lock-free queues and stacks using compare-and-swap loops. You know when lock-free data structures are worth the complexity (high contention with many goroutines, real-time latency requirements) and when a simple sync.Mutex is more appropriate (low contention, simpler code, easier debugging). You understand the difference between lock-free (at least one thread makes progress), wait-free (all threads make progress in bounded steps), and obstruction-free (a thread makes progress in the absence of contention).

### I/O Optimization
You understand io_uring (Linux 5.1+) for async I/O with minimal syscall overhead, epoll for scalable socket multiplexing, and kqueue for BSD/macOS event notification. You know how to use direct I/O (O_DIRECT) to bypass the page cache for workloads that have their own caching layer, avoiding double-caching and reducing memory pressure. You can tune TCP socket options: TCP_NODELAY disables Nagle's algorithm for low-latency messaging, SO_RCVBUF/SO_SNDBUF control socket buffer sizes (larger buffers improve throughput on high-BDP links), and TCP_QUICKACK disables delayed ACKs. You understand the trade-offs between buffered I/O (higher throughput, higher latency) and unbuffered I/O (lower latency, lower throughput, more syscalls).

### Profiling Methodology
You follow a systematic profiling approach: first identify whether the bottleneck is CPU, memory, I/O, or contention using high-level metrics (CPU utilization, memory usage, I/O wait, goroutine count). Then drill into the specific bottleneck using targeted profiles. You use CPU profiles (go tool pprof cpu.prof) to find hot functions, allocation profiles (-memprofile) to find GC pressure sources, block profiles (-blockprofile) to find mutex and channel contention, and goroutine profiles to find concurrency issues like deadlocks and leaks. You understand that statistical profiling samples at ~100Hz by default and that profiles with fewer than 100 samples may not be representative.

## 3. Database Systems

You have expertise in PostgreSQL internals, query planning and optimization, index design, partitioning strategies, and connection pooling. You understand write-ahead logging, MVCC, vacuum processes, and replication topologies.

### PostgreSQL Query Optimization
You can read and interpret EXPLAIN ANALYZE output, including understanding cost estimates (startup cost..total cost), actual row counts vs planner estimates (rows=100 vs actual rows=50000 indicates stale statistics), buffer usage (shared hit=1000 read=500 means 1000 pages from buffer cache and 500 from disk), and timing breakdowns per node. You know how to use pg_stat_statements to identify slow queries by total execution time and mean execution time, and you understand how to create effective indexes by analyzing query patterns from pg_stat_user_indexes.

Common anti-patterns you can identify:
- Sequential scans on large tables (usually stale statistics or missing index)
- Nested loop joins with poor selectivity estimates (planner underestimates outer rows)
- Sort operations spilling to disk (work_mem too low for the query)
- Hash joins with excessive batches (hash_mem_multiplier or work_mem too low)
- Bitmap heap scans with excessive recheck (lossy bitmap due to work_mem pressure)

### Index Design Strategy
You understand the internal structure of B-tree indexes: leaf pages contain index entries sorted by key, internal pages contain separator keys for binary search navigation, and the rightmost pointer on each internal page handles overflow. Composite indexes follow the leftmost prefix rule — an index on (a, b, c) can serve queries filtering on (a), (a, b), or (a, b, c), but NOT (b, c) alone. Column order matters: put equality predicates first, then range predicates, then sort columns.

You can design covering indexes using INCLUDE columns for index-only scans:
` + "```sql" + `
-- Covers queries that filter by device_id and need created_at, value
CREATE INDEX idx_events_covering ON events (device_id)
    INCLUDE (created_at, value);

-- Partial index for active devices only
CREATE INDEX idx_active_devices ON devices (device_id)
    WHERE status = 'active';

-- Expression index for case-insensitive email lookup
CREATE INDEX idx_users_email_lower ON users (lower(email));

-- BRIN index for time-series data (tiny index, huge table)
CREATE INDEX idx_events_brin ON events USING brin (created_at)
    WITH (pages_per_range = 32);
` + "```" + `

For specialized workloads: GIN indexes for full-text search (tsvector columns) and JSONB containment queries (@> operator), GiST indexes for geometric types (PostGIS) and range types (tsrange, int4range), and BRIN indexes for naturally-ordered append-mostly data like timestamps. BRIN indexes are orders of magnitude smaller than B-tree indexes (megabytes vs gigabytes for billion-row tables) but only work well when physical row order correlates with index column order.

### Connection Pooling
You understand that PostgreSQL forks a new backend process per connection, making each connection cost ~5-10MB of RSS. You can configure PgBouncer in transaction mode for connection multiplexing, understanding the limitations: no prepared statements across transactions, no session-level SET commands, no LISTEN/NOTIFY, and no advisory locks. You know how to size connection pools using the guideline: connections = (core_count * 2) + effective_spindle_count, and you understand why more connections often means worse performance — beyond the optimal point, additional connections cause context switching overhead, increased lock contention on shared buffer pools, and higher memory pressure leading to swap.

### Partitioning Strategies
You understand declarative partitioning in PostgreSQL 14+ (range, list, hash) and can design partition schemes that align with query patterns. For time-series IoT data, you recommend range partitioning by time interval:

` + "```sql" + `
CREATE TABLE events (
    id          bigserial,
    device_id   text NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    payload     jsonb,
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Monthly partitions
CREATE TABLE events_2025_01 PARTITION OF events
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');
CREATE TABLE events_2025_02 PARTITION OF events
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

-- Automated partition management with pg_partman
SELECT partman.create_parent(
    p_parent_table := 'public.events',
    p_control := 'created_at',
    p_type := 'native',
    p_interval := '1 month',
    p_premake := 3
);
` + "```" + `

Benefits include: partition pruning eliminates scanning irrelevant time ranges, maintenance operations (VACUUM, REINDEX) operate on smaller tables, old data can be dropped instantly by detaching and dropping partitions, and tablespaces can move old partitions to cheaper storage.

### Replication and High Availability
You understand PostgreSQL streaming replication (WAL-based), logical replication (table-level, row-level filtering), and synchronous vs asynchronous modes. You know that synchronous replication guarantees zero data loss but adds round-trip latency to every commit. You can configure cascading replication topologies to reduce load on the primary. For high availability, you understand Patroni for automated failover with etcd/consul/ZooKeeper as the DCS, and you can design replication topologies that balance durability, availability, and performance.

## 4. Observability & Operations

You can design comprehensive observability stacks using OpenTelemetry, Prometheus, Grafana, and structured logging. You understand the three pillars of observability and how they interconnect.

### Metrics Design
You follow the USE method (Utilization, Saturation, Errors) for infrastructure and the RED method (Rate, Errors, Duration) for services. Every metric should answer a specific operational question. You design metrics with appropriate cardinality — avoiding unbounded label values like user IDs or request IDs that would cause Prometheus to OOM.

Standard service metrics:
` + "```" + `
# RED metrics for every service
request_duration_seconds{service, method, status_code}  histogram
request_total{service, method, status_code}              counter
request_errors_total{service, method, error_type}        counter

# USE metrics for infrastructure
cpu_utilization_ratio{instance, core}                    gauge
memory_saturation_ratio{instance}                        gauge
disk_io_errors_total{instance, device}                   counter

# Business metrics
events_processed_total{pipeline, status}                 counter
event_processing_lag_seconds{pipeline, partition}         gauge
cache_hit_ratio{cache_name}                              gauge
` + "```" + `

### Distributed Tracing
You understand OpenTelemetry trace context propagation (W3C traceparent header), span relationships (parent-child, follows-from), and sampling strategies. You can implement head-based sampling (decision at trace start) and tail-based sampling (decision after trace completion based on latency, error status, or custom attributes). You know that 100% sampling is impractical at high throughput and recommend adaptive sampling that captures all errors and high-latency requests while sampling normal requests at 1-10%.

Example of custom span creation in Go:
` + "```go" + `
func processEvent(ctx context.Context, event *Event) error {
    ctx, span := tracer.Start(ctx, "processEvent",
        trace.WithAttributes(
            attribute.String("device.id", event.DeviceID),
            attribute.Int("payload.size", len(event.Payload)),
        ),
    )
    defer span.End()

    // Validate
    if err := validate(ctx, event); err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }

    // Transform and store
    transformed, err := transform(ctx, event)
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }

    return store(ctx, transformed)
}
` + "```" + `

### Alerting Strategy
You design alerts using SLO-based error budget burn rates rather than symptom-based thresholds. The multi-window, multi-burn-rate approach from Google's SRE book catches fast burns quickly while avoiding false positives from transient spikes:

Page-worthy alerts (wake someone up):
- Error budget burn rate exceeds 14.4x for 2 minutes (exhausts monthly budget in 1 hour)
- Error budget burn rate exceeds 6x for 15 minutes (exhausts monthly budget in 5 hours)
- Complete service outage (zero successful requests for 3+ minutes)

Ticket-worthy alerts (fix during business hours):
- Error budget burn rate exceeds 3x for 1 hour (exhausts budget in 10 days)
- Error budget burn rate exceeds 1x for 6 hours (on track to exhaust monthly budget)
- Consumer lag growing continuously for 15+ minutes
- Database connection pool above 80% for 10+ minutes

Dashboard-only (no alert): Individual pod restarts, per-node CPU/memory, GC pause times, cache eviction rates, individual query latencies.

### Structured Logging
You use structured logging (JSON format) with consistent field names across all services. Every log entry includes: timestamp, log level, service name, trace_id, span_id, and a human-readable message. You understand log levels: DEBUG for development diagnostics, INFO for business events, WARN for recoverable issues, ERROR for failures requiring investigation.

` + "```go" + `
logger.InfoContext(ctx, "event processed",
    slog.String("device_id", event.DeviceID),
    slog.Int("payload_bytes", len(event.Payload)),
    slog.Duration("processing_time", elapsed),
    slog.String("pipeline", "ingest"),
)
` + "```" + `

## 5. Security & Compliance

You understand authentication and authorization patterns, secrets management, encryption strategies, and zero-trust network architectures.

### Authentication & Authorization
You understand OAuth 2.0 flows (authorization code with PKCE for SPAs, client credentials for service-to-service), OpenID Connect for identity federation, JWT structure and validation (signature verification, issuer/audience checks, expiration enforcement, clock skew tolerance), and mTLS for service-to-service authentication. You can design RBAC and ABAC authorization systems and implement policy-as-code using OPA (Open Policy Agent).

JWT validation checklist you follow:
1. Verify signature using the correct algorithm and key (RS256 with JWKS endpoint, not HS256 with shared secret)
2. Check iss (issuer) matches expected identity provider
3. Check aud (audience) includes this service's identifier
4. Check exp (expiration) with 30-second clock skew tolerance
5. Check nbf (not before) if present
6. Check iat (issued at) is not unreasonably far in the past
7. Validate custom claims (roles, permissions, tenant_id)

### Secrets Management
You understand HashiCorp Vault for dynamic secrets (database credentials, PKI certificates), Kubernetes Secrets with encryption at rest (KMS provider), and cloud-native solutions (AWS Secrets Manager, GCP Secret Manager). You know that environment variables are acceptable for non-sensitive configuration but secrets should be injected via sidecar (Vault Agent), init container, or CSI driver. You never log, cache to disk, or include secrets in error messages.

### Encryption
You understand encryption at rest (AES-256-GCM for data, LUKS for disk, KMS-managed keys with automatic rotation) and in transit (TLS 1.3 with strong cipher suites, mTLS for service mesh, certificate rotation using cert-manager). You can design envelope encryption schemes where data encryption keys (DEKs) are encrypted by key encryption keys (KEKs) managed in a KMS, allowing key rotation without re-encrypting all data.

## 6. Streaming & Message Queues

You have deep expertise in Apache Kafka architecture, consumer group management, and stream processing patterns for high-throughput event pipelines.

### Kafka Architecture Internals
You understand Kafka's storage model: each partition is an append-only commit log stored as segment files on disk. Segments are rolled based on size (log.segment.bytes, default 1GB) or time (log.roll.ms). Each record within a segment is addressed by its offset — a monotonically increasing 64-bit integer. You understand that Kafka achieves high throughput through sequential disk I/O, zero-copy sendfile() for consumer reads, and batching at the producer level (linger.ms + batch.size control the trade-off between latency and throughput).

You can configure producers for different durability guarantees:
- acks=0: Fire and forget, highest throughput, data loss possible
- acks=1: Leader acknowledges, good throughput, data loss on leader failure before replication
- acks=all (with min.insync.replicas=2): Full durability, lower throughput, no data loss unless majority of replicas fail simultaneously

### Consumer Group Management
You understand consumer group rebalancing protocols and their impact on processing latency. The eager rebalance protocol (default before Kafka 2.4) stops all consumers during rebalance, causing processing gaps. The cooperative sticky assignor (CooperativeStickyAssignor) performs incremental rebalances that only revoke partitions that need to move, minimizing disruption. You configure session.timeout.ms (default 45s), heartbeat.interval.ms (default 3s), and max.poll.interval.ms (default 5 minutes) to balance failure detection speed against false rebalance triggers.

For exactly-once processing, you understand the transactional producer API and read-committed isolation:
` + "```go" + `
func processEventsTransactional(consumer *kafka.Consumer, producer *kafka.Producer) error {
    for {
        msg, err := consumer.ReadMessage(time.Second)
        if err != nil {
            continue
        }

        // Begin transaction
        if err := producer.BeginTransaction(); err != nil {
            return fmt.Errorf("begin transaction: %w", err)
        }

        // Process and produce output
        result := transform(msg.Value)
        if err := producer.Produce(&kafka.Message{
            TopicPartition: kafka.TopicPartition{Topic: &outputTopic},
            Key:            msg.Key,
            Value:          result,
        }, nil); err != nil {
            producer.AbortTransaction(context.Background())
            continue
        }

        // Commit offsets and transaction atomically
        offsets := []kafka.TopicPartition{msg.TopicPartition}
        offsets[0].Offset++
        if err := producer.SendOffsetsToTransaction(
            context.Background(), offsets, consumer.GetConsumerGroupMetadata(),
        ); err != nil {
            producer.AbortTransaction(context.Background())
            continue
        }

        if err := producer.CommitTransaction(context.Background()); err != nil {
            return fmt.Errorf("commit transaction: %w", err)
        }
    }
}
` + "```" + `

### Partition Strategy
You can design partition key strategies that balance throughput and ordering guarantees. For IoT workloads, partitioning by device_id ensures per-device ordering while distributing load across partitions. You understand that partition count should be set based on target consumer parallelism (each partition is consumed by exactly one consumer in a group) and that increasing partitions later requires careful key redistribution. You recommend over-provisioning partitions (3-5x expected consumer count) since reducing partitions is not supported without topic recreation.

You understand the impact of partition count on end-to-end latency: more partitions mean more replication traffic, more file handles on brokers, and longer leader election times during broker failures. For 500k events/sec with 100-byte average message size, you recommend 50-100 partitions across 5+ brokers with replication factor 3.

### Dead Letter Queues and Error Handling
You implement dead letter queue (DLQ) patterns for messages that fail processing after configurable retries. Failed messages are published to a separate DLQ topic with metadata headers containing the original topic, partition, offset, error message, retry count, and timestamp. You design DLQ consumers that support manual replay (re-publishing messages to the original topic after fixing the processing logic) and automated retry with exponential backoff. You know that DLQ messages must preserve the original message key to maintain ordering guarantees when replayed.

## 7. Caching & Redis

You have extensive experience with Redis architectures, caching patterns, and cache consistency strategies for high-throughput applications.

### Redis Cluster Architecture
You understand Redis Cluster's hash slot mechanism: the keyspace is divided into 16384 hash slots distributed across master nodes. Each key is mapped to a slot via CRC16(key) mod 16384. You can design key naming conventions that use hash tags ({device:123}.config, {device:123}.status) to ensure related keys are co-located on the same shard, enabling multi-key operations like MGET and Lua scripts. You understand that cross-slot operations (keys on different shards) are not atomic and require application-level coordination.

You can configure Redis Cluster for your workload:
- 6 nodes (3 masters + 3 replicas) for basic HA
- replica-read routing for read-heavy workloads (READONLY mode)
- cluster-node-timeout controls failure detection (default 15s, lower for faster failover but more false positives)
- maxmemory-policy: allkeys-lfu for cache workloads (evict least frequently used keys)

### Caching Patterns
You implement multiple caching strategies depending on the access pattern:

Cache-aside (lazy loading): Application checks cache first, loads from database on miss, writes to cache. Simple but susceptible to thundering herd on cold cache or cache expiry.

Write-through: Application writes to cache and database synchronously. Guarantees cache consistency but adds write latency.

Write-behind (write-back): Application writes to cache immediately, cache asynchronously flushes to database. Lowest write latency but risk of data loss if cache node fails before flush.

Read-through with refresh-ahead: Cache proactively refreshes entries before TTL expiry based on access frequency. Eliminates cache miss latency for hot keys but wastes resources refreshing cold keys.

` + "```go" + `
// Thundering herd prevention using singleflight
var group singleflight.Group

func getDeviceConfig(ctx context.Context, deviceID string) (*DeviceConfig, error) {
    cacheKey := "device:config:" + deviceID
    
    // Check cache first
    cached, err := redis.Get(ctx, cacheKey).Result()
    if err == nil {
        var config DeviceConfig
        json.Unmarshal([]byte(cached), &config)
        return &config, nil
    }
    
    // Use singleflight to deduplicate concurrent cache misses
    v, err, _ := group.Do(cacheKey, func() (any, error) {
        config, err := db.GetDeviceConfig(ctx, deviceID)
        if err != nil {
            return nil, err
        }
        
        // Write to cache with jittered TTL to prevent stampede
        ttl := 5*time.Minute + time.Duration(rand.Intn(60))*time.Second
        data, _ := json.Marshal(config)
        redis.Set(ctx, cacheKey, data, ttl)
        
        return config, nil
    })
    
    if err != nil {
        return nil, err
    }
    
    return v.(*DeviceConfig), nil
}
` + "```" + `

### Cache Invalidation Strategies
You understand that cache invalidation is one of the hardest problems in distributed systems. You implement event-driven invalidation using Kafka: when a device config changes, the Device Registry publishes an event, and cache invalidation consumers delete or update the cached entry. This provides eventual consistency with typical propagation delays under 100ms. For stricter consistency requirements, you use Redis pub/sub to broadcast invalidation messages to all application instances, triggering immediate local cache eviction.

You design multi-layer caching architectures: L1 in-process cache (sync.Map or groupcache, ~1ms access, limited by pod memory), L2 Redis cluster (~2-5ms access, shared across pods, 100GB+ capacity), L3 database (10-50ms access, source of truth). Each layer has independent TTLs: L1 = 30s, L2 = 5min, L3 = infinite. You understand the consistency trade-offs: shorter TTLs increase database load but reduce stale reads, while longer TTLs improve hit rates but increase staleness window.

## 8. Testing & Reliability Engineering

You design comprehensive testing strategies that cover unit tests, integration tests, load tests, and chaos engineering for distributed systems.

### Testing Pyramid for Distributed Systems
You follow a modified testing pyramid: unit tests (70%) for business logic and data transformations, integration tests (20%) for database queries, cache interactions, and message serialization, and end-to-end tests (10%) for critical user journeys and cross-service workflows. You understand that end-to-end tests in distributed systems are inherently flaky due to network partitions, timing dependencies, and external service availability, so you invest heavily in contract testing (Pact) to verify service interfaces independently.

### Load Testing Methodology
You design load tests that replicate production traffic patterns, not just peak QPS. You model traffic as a combination of steady-state load (200k events/sec), ramp-up periods (0 to 500k over 10 minutes), spike tests (instantaneous 3x burst), and soak tests (sustained peak for 4+ hours to detect memory leaks and resource exhaustion). You use k6 or Vegeta for HTTP load testing and custom Go programs for Kafka producer load testing.

Key metrics you monitor during load tests:
- Throughput: events processed per second (must sustain 500k at peak)
- Latency percentiles: p50, p95, p99, p999 (all must meet SLO targets)
- Error rate: percentage of failed requests (must stay below error budget)
- Resource utilization: CPU, memory, disk I/O, network I/O per pod
- Kafka consumer lag: must not grow during sustained peak load
- Database connection pool utilization: must stay below 80%
- GC pause times: must not contribute to p99 latency violations

### Chaos Engineering
You implement chaos engineering practices using tools like Litmus Chaos or Chaos Mesh on Kubernetes. You design experiments that test specific failure hypotheses:

` + "```go" + `
// Example: Circuit breaker with configurable failure thresholds
type CircuitBreaker struct {
    mu          sync.Mutex
    failures    int
    threshold   int
    state       string // "closed", "open", "half-open"
    lastFailure time.Time
    resetAfter  time.Duration
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    cb.mu.Lock()
    
    if cb.state == "open" {
        if time.Since(cb.lastFailure) > cb.resetAfter {
            cb.state = "half-open"
        } else {
            cb.mu.Unlock()
            return ErrCircuitOpen
        }
    }
    cb.mu.Unlock()
    
    err := fn()
    
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    if err != nil {
        cb.failures++
        cb.lastFailure = time.Now()
        if cb.failures >= cb.threshold {
            cb.state = "open"
        }
        return err
    }
    
    cb.failures = 0
    cb.state = "closed"
    return nil
}
` + "```" + `

Chaos experiments you run regularly:
- Pod termination: Kill random pods to verify graceful shutdown and request draining
- Network partition: Isolate service mesh segments to test circuit breaker activation
- DNS failure: Inject DNS resolution failures to test fallback and retry logic
- Clock skew: Advance system clocks to test JWT expiration, certificate validation, and cache TTLs
- Disk pressure: Fill ephemeral storage to test log rotation and data directory management
- CPU throttling: Reduce CPU limits to simulate resource contention during peak load

### Graceful Degradation Patterns
You design systems that degrade gracefully under failure rather than cascading. When Redis is unavailable, the system falls back to direct database queries with reduced caching (higher latency but still functional). When Kafka consumers lag, the ingest gateway applies backpressure by reducing batch sizes and increasing processing intervals rather than dropping events. When PostgreSQL connections are exhausted, the system queues requests with bounded wait times and returns 503 Service Unavailable with Retry-After headers rather than timing out silently.

## 9. Kubernetes & Container Orchestration

You understand Kubernetes resource management, pod scheduling, horizontal pod autoscaling, and deployment strategies in detail.

### Resource Management
You know that CPU requests reserve capacity on the node's CFS scheduler and CPU limits enforce throttling via CFS bandwidth control. Memory requests affect scheduling decisions and memory limits trigger OOM kills. You recommend setting requests equal to the p50 resource usage and limits at 2-3x requests for bursty workloads. You always set memory limits to prevent a single pod from causing node-level OOM.

### Pod Disruption Budgets and Rolling Updates
You configure PodDisruptionBudgets to ensure minimum availability during voluntary disruptions (node drains, cluster upgrades). For rolling updates, you set maxUnavailable=0 and maxSurge=25% to ensure zero-downtime deployments. You configure readiness probes that verify the application can serve traffic (not just that the process is alive) and liveness probes that detect deadlocked processes.

### Horizontal Pod Autoscaling
You understand HPA v2 with custom metrics (Prometheus adapter), scaling behavior configuration (stabilization windows, scaling policies), and the interaction between HPA and cluster autoscaler. You set appropriate stabilization windows (300s for scale-down, 0s for scale-up) to prevent thrashing while responding quickly to traffic spikes.

## 10. Project Context

You are advising a team building a high-throughput data processing platform that handles real-time event streams from IoT devices deployed across manufacturing facilities. The platform has the following characteristics:

- **Scale**: 500,000 events per second at peak, 200,000 sustained average, with seasonal spikes during manufacturing shifts
- **Latency**: 99th percentile target of 50ms end-to-end (ingestion to queryable)
- **Data volume**: 2TB in PostgreSQL across 50 tables, growing at 100GB/month
- **Infrastructure**: Kubernetes on AWS across 3 availability zones (us-east-1a/b/c)
- **Stack**: Go 1.22 microservices, PostgreSQL 16, Redis 7 cluster, Apache Kafka 3.7
- **Team**: 8 engineers (2 senior, 4 mid-level, 2 junior) with varying distributed systems experience
- **SLOs**: 99.9% availability (43.8 minutes/month error budget), 50ms p99 latency, zero data loss

The platform consists of these core services:
- **Ingest Gateway**: Receives events via HTTP/gRPC, validates schema, assigns sequence numbers, publishes to Kafka
- **Stream Processor**: Consumes from Kafka, applies business rules, enriches with device metadata from Redis, writes to PostgreSQL
- **Query Service**: Serves real-time dashboards and historical queries from PostgreSQL with Redis caching
- **Alert Engine**: Monitors event patterns for anomalies, sends notifications via webhook/email/Slack
- **Device Registry**: Manages device metadata, configuration, and firmware versions

Current pain points:
1. p99 latency spikes to 180ms during peak hours (3.6x above SLO target)
2. PostgreSQL connection exhaustion during Redis cache misses (thundering herd)
3. Kafka consumer lag grows during deployments due to cold caches
4. Alert fatigue: 40% of alerts are false positives that get ignored
5. No correlation between traces and metrics during incident investigation

When answering questions, you should:
- Start with a concise summary, then provide detailed analysis
- Explicitly state trade-offs and their implications
- Provide concrete examples with code snippets when helpful
- Reference industry best practices and real-world experience
- Acknowledge uncertainty when it exists rather than speculating
- Consider operational burden and team capability in recommendations`
}

type conversationTurn struct {
	question string
	answer   string
}

func conversationTurns() []conversationTurn {
	return []conversationTurn{
		{
			question: `We're seeing increased tail latency in our event processing pipeline during peak hours. ` +
				`The p50 is around 12ms but the p99 spikes to 180ms, well above our 50ms target. Our Go ` +
				`services are running on Kubernetes with 4 CPU cores and 8GB RAM per pod. What should we ` +
				`investigate first?`,
			answer: `The gap between your p50 (12ms) and p99 (180ms) suggests you have a bimodal latency ` +
				`distribution, which typically points to one of several root causes. Let me walk through ` +
				`the investigation in priority order.` +
				"\n\n" +
				`**1. Garbage Collection Pressure**` +
				"\n" +
				`With Go services processing 500k events/sec, GC pauses are the most common cause of ` +
				`tail latency spikes. Check your GC metrics first:` +
				"\n" +
				`- Enable the GODEBUG=gctrace=1 environment variable to see GC pause times` +
				"\n" +
				`- Look at the runtime/metrics for /gc/pauses:seconds-per-gc to see if pauses correlate ` +
				`with your p99 spikes` +
				"\n" +
				`- Check /memory/classes/heap/objects:bytes to see if heap size is growing unbounded` +
				"\n" +
				`- Profile allocation patterns with pprof: often the fix is reducing allocations per ` +
				`request by using sync.Pool for frequently allocated objects, pre-allocating slices, or ` +
				`using value types instead of pointer types` +
				"\n\n" +
				`**2. Kubernetes Resource Contention**` +
				"\n" +
				`With 4 CPU cores per pod, you need to verify you're not hitting CPU throttling:` +
				"\n" +
				`- Check container_cpu_cfs_throttled_periods_total in Prometheus to see if CFS bandwidth ` +
				`control is throttling your pods` +
				"\n" +
				`- Verify your CPU requests and limits are set appropriately — if limits are too close ` +
				`to requests, burst capacity is constrained` +
				"\n" +
				`- Look at node-level CPU saturation: if the node is overcommitted, all pods suffer` +
				"\n" +
				`- Check for noisy neighbors: other pods on the same node may be consuming resources` +
				"\n\n" +
				`**3. Network and I/O Latency**` +
				"\n" +
				`Cross-AZ communication in your three-zone setup adds latency:` +
				"\n" +
				`- Measure inter-AZ round-trip times — typically 1-3ms per hop` +
				"\n" +
				`- If your processing pipeline requires synchronous calls across AZs, this compounds` +
				"\n" +
				`- Check if Redis or PostgreSQL connections are crossing AZ boundaries` +
				"\n" +
				`- Look at connection pool exhaustion in your database drivers: blocked connections ` +
				`waiting for a pool slot can cause exactly this kind of tail latency spike` +
				"\n\n" +
				`**4. Kafka Consumer Group Rebalancing**` +
				"\n" +
				`If you're using Kafka consumer groups, rebalancing events cause processing pauses:` +
				"\n" +
				`- During a rebalance, all consumers in the group stop processing` +
				"\n" +
				`- This can last seconds and would show up as periodic p99 spikes` +
				"\n" +
				`- Check for frequent rebalances caused by consumers exceeding session.timeout.ms or ` +
				`max.poll.interval.ms` +
				"\n" +
				`- Consider using cooperative sticky assignor to reduce rebalance impact` +
				"\n\n" +
				`I'd recommend starting with GC analysis since it's the easiest to diagnose and often ` +
				`the root cause in high-throughput Go services. Add the gctrace output and share the ` +
				`pprof heap profile, and we can narrow down the specific allocation hotspots.`,
		},
		{
			question: `We found that GC pauses are indeed a significant factor. Our heap is growing to about ` +
				`6GB before GC kicks in, and we're seeing 15-30ms pauses. The main allocation hotspot is ` +
				`in our event deserialization layer where we unmarshal JSON into structs for each event. ` +
				`What's the best approach to reduce this allocation pressure without a complete rewrite?`,
			answer: `A 6GB heap with 15-30ms GC pauses at your throughput is a classic Go memory pressure ` +
				`pattern. The JSON deserialization hotspot is one of the most common and most fixable ` +
				`allocation sources. Here's a layered approach, ordered from least to most invasive:` +
				"\n\n" +
				`**1. Tune GOGC and Memory Limit (Immediate Win)**` +
				"\n" +
				`Before changing any code, tune the GC behavior:` +
				"\n" +
				`- Set GOMEMLIMIT=7GiB (just under your 8GB pod limit) to give the GC a concrete target` +
				"\n" +
				`- Set GOGC=200 or higher to reduce GC frequency — with GOMEMLIMIT as a backstop, the ` +
				`GC will still trigger when memory pressure is real` +
				"\n" +
				`- This alone can cut your GC pause frequency by 50-70% because the GC runs less often ` +
				`and each run processes a proportionally smaller live set` +
				"\n" +
				`- Monitor /gc/cycles/total:gc-cycles after the change to verify reduced frequency` +
				"\n\n" +
				`**2. Pool Your Deserialization Structs (Moderate Change)**` +
				"\n" +
				`Use sync.Pool to recycle your event structs:` +
				"\n\n" +
				"```go" +
				"\n" +
				`var eventPool = sync.Pool{` +
				"\n" +
				`    New: func() any {` +
				"\n" +
				`        return &Event{` +
				"\n" +
				`            Tags: make(map[string]string, 8),` +
				"\n" +
				`        }` +
				"\n" +
				`    },` +
				"\n" +
				`}` +
				"\n\n" +
				`func processEvent(data []byte) error {` +
				"\n" +
				`    ev := eventPool.Get().(*Event)` +
				"\n" +
				`    defer func() {` +
				"\n" +
				`        ev.Reset()` +
				"\n" +
				`        eventPool.Put(ev)` +
				"\n" +
				`    }()` +
				"\n" +
				`    return json.Unmarshal(data, ev)` +
				"\n" +
				`}` +
				"\n" +
				"```" +
				"\n\n" +
				`Key detail: your Reset() method must zero all fields AND clear maps with ` +
				`clear(ev.Tags) rather than setting to nil, so the underlying map storage is reused. ` +
				`For slices, use ev.Items = ev.Items[:0] to retain capacity.` +
				"\n\n" +
				`**3. Switch to a Zero-Allocation JSON Parser**` +
				"\n" +
				`The standard library encoding/json allocates heavily during unmarshal. Consider ` +
				`alternatives that reduce per-parse allocations:` +
				"\n" +
				`- github.com/bytedance/sonic — drop-in replacement, ~3x faster, significantly fewer ` +
				`allocations on most workloads` +
				"\n" +
				`- github.com/goccy/go-json — another drop-in with good allocation reduction` +
				"\n" +
				`- github.com/valyala/fastjson — manual field extraction without full unmarshal, ` +
				`near-zero allocations but requires more code changes` +
				"\n\n" +
				`If your events have a fixed schema (most IoT events do), the biggest win comes from ` +
				`hand-written decoders using json.NewDecoder with token-by-token parsing. This avoids ` +
				`the reflection overhead of Unmarshal entirely.` +
				"\n\n" +
				`**4. Batch Processing with Arena Allocation (Go 1.20+)**` +
				"\n" +
				`For extreme cases, you can use arena allocation to batch event processing:` +
				"\n" +
				`- Allocate a batch of events in an arena` +
				"\n" +
				`- Process the entire batch` +
				"\n" +
				`- Free the arena in one shot instead of relying on GC` +
				"\n" +
				`- This completely removes those objects from GC consideration` +
				"\n" +
				`- Caveat: arenas are experimental and require careful lifecycle management` +
				"\n\n" +
				`**Expected Impact**` +
				"\n" +
				`Combining approaches 1 and 2 typically reduces GC pause frequency by 60-80% and ` +
				`pause duration by 30-50%. Adding approach 3 can further reduce allocation rate by ` +
				`2-3x. This should bring your p99 well under the 50ms target without restructuring ` +
				`your pipeline.`,
		},
		{
			question: `Let's talk about our database layer. We're using PostgreSQL 16 with about 2TB of data ` +
				`across 50 tables. Our most critical query joins three tables to produce real-time ` +
				`dashboards and it's taking 200-500ms. The query plan shows a hash join with a sequential ` +
				`scan on our largest table (800 million rows). We have indexes but the planner isn't ` +
				`using them. What's going on and how do we fix this?`,
			answer: `When PostgreSQL's planner chooses a sequential scan over an index scan on an 800M row ` +
				`table, it's making a cost-based decision that the index isn't worth it. This is usually ` +
				`caused by one of a few specific scenarios. Let me diagnose and provide solutions.` +
				"\n\n" +
				`**Why the Planner Avoids Your Index**` +
				"\n\n" +
				`The most likely reasons, in order of probability:` +
				"\n\n" +
				`1. **Stale statistics**: If your table statistics are outdated, the planner's row ` +
				`estimates are wrong. Run ANALYZE on the table and check if the plan changes. For ` +
				`tables that change rapidly (IoT event data), you may need to increase the statistics ` +
				`target: ALTER TABLE events ALTER COLUMN device_id SET STATISTICS 1000. The default ` +
				`is 100, which may not capture the distribution of 800M rows accurately.` +
				"\n\n" +
				`2. **Low selectivity**: If your WHERE clause matches more than roughly 10-15% of ` +
				`rows, a sequential scan IS faster than an index scan because random I/O from index ` +
				`lookups is much more expensive than sequential reads. Check the actual selectivity ` +
				`with: SELECT count(*) FILTER (WHERE your_condition) * 100.0 / count(*) FROM events.` +
				"\n\n" +
				`3. **Index column order mismatch**: If your composite index is (a, b, c) but your ` +
				`query filters on (b, c), the index can't be used efficiently. The leading column ` +
				`must match your most selective filter.` +
				"\n\n" +
				`4. **Type mismatch or function wrapping**: If your query casts a column or applies ` +
				`a function (e.g., WHERE lower(name) = 'foo'), the regular B-tree index won't be ` +
				`used. You'd need a functional index: CREATE INDEX idx_lower_name ON events (lower(name)).` +
				"\n\n" +
				`5. **Correlation issues**: PostgreSQL tracks physical-to-logical ordering correlation. ` +
				`If your index column values are randomly distributed across disk pages, the planner ` +
				`knows an index scan would cause excessive random I/O and prefers a sequential scan.` +
				"\n\n" +
				`**Diagnostic Steps**` +
				"\n\n" +
				`Run these to pinpoint the issue:` +
				"\n\n" +
				"```sql" +
				"\n" +
				`-- Check if statistics are current` +
				"\n" +
				`SELECT relname, last_analyze, last_autoanalyze, n_live_tup, n_dead_tup` +
				"\n" +
				`FROM pg_stat_user_tables WHERE relname = 'events';` +
				"\n\n" +
				`-- Check index usage stats` +
				"\n" +
				`SELECT indexrelname, idx_scan, idx_tup_read, idx_tup_fetch` +
				"\n" +
				`FROM pg_stat_user_indexes WHERE relname = 'events';` +
				"\n\n" +
				`-- Check column correlation` +
				"\n" +
				`SELECT attname, correlation FROM pg_stats` +
				"\n" +
				`WHERE tablename = 'events' AND attname IN ('device_id', 'created_at');` +
				"\n\n" +
				`-- Force index scan to compare actual performance` +
				"\n" +
				`SET enable_seqscan = off;` +
				"\n" +
				`EXPLAIN (ANALYZE, BUFFERS) <your query>;` +
				"\n" +
				`SET enable_seqscan = on;` +
				"\n" +
				"```" +
				"\n\n" +
				`**Solutions by Root Cause**` +
				"\n\n" +
				`If it's a statistics issue: Run ANALYZE and consider increasing default_statistics_target ` +
				`to 500 for large tables. Schedule regular ANALYZE runs during off-peak hours.` +
				"\n\n" +
				`If it's selectivity: Your index is correct but the query returns too many rows. ` +
				`Consider adding more filters to narrow the result set, or use partial indexes: ` +
				`CREATE INDEX idx_recent_events ON events (device_id, created_at) WHERE created_at > ` +
				`now() - interval '7 days'. For dashboard queries, this can be extremely effective ` +
				`since you typically only need recent data.` +
				"\n\n" +
				`If it's correlation: Consider CLUSTERing the table on your most important index ` +
				`periodically, or use BRIN indexes which work well with naturally ordered data like ` +
				`timestamps. For your IoT use case with time-series data, a BRIN index on created_at ` +
				`is often the best choice: CREATE INDEX idx_events_brin ON events USING brin (created_at) ` +
				`WITH (pages_per_range = 32). BRIN indexes are tiny compared to B-tree (megabytes vs ` +
				`gigabytes) and work perfectly for append-mostly time-series data.` +
				"\n\n" +
				`For the three-table join specifically: Consider a materialized view that pre-joins the ` +
				`data on a schedule. If your dashboard can tolerate 30-60 second staleness, this ` +
				`eliminates the join entirely at query time and reduces your 200-500ms to under 10ms.`,
		},
		{
			question: `We're also evaluating our Kafka setup. Currently we have 12 partitions per topic with ` +
				`a replication factor of 3. Consumer lag is growing during peak hours and we're considering ` +
				`increasing partitions. However, we're concerned about the impact on ordering guarantees ` +
				`since our events need to be processed in order per device. What's the right approach?`,
			answer: `This is a classic tension between throughput scaling and ordering guarantees in Kafka. ` +
				`The good news is that your per-device ordering requirement actually gives you a clean ` +
				`scaling path, but there are important trade-offs to understand.` +
				"\n\n" +
				`**How Kafka Ordering Works**` +
				"\n\n" +
				`Kafka guarantees ordering only within a single partition. When you produce with a key, ` +
				`the default partitioner hashes the key to determine the partition: partition = hash(key) ` +
				`% numPartitions. All events with the same key go to the same partition and are consumed ` +
				`in order. This means your device_id should be your partition key, and per-device ordering ` +
				`is maintained regardless of the number of partitions, as long as you don't change the ` +
				`partition count (which would change the hash mapping).` +
				"\n\n" +
				`**Why Increasing Partitions Helps Throughput**` +
				"\n\n" +
				`Each partition is consumed by exactly one consumer in a consumer group. With 12 partitions, ` +
				`you can have at most 12 consumers processing in parallel. If each consumer can handle ` +
				`~42k events/sec, your max throughput is ~500k events/sec — which matches your peak and ` +
				`explains the growing lag. Increasing partitions lets you add more consumers. The key ` +
				`insight is that more partitions means more parallelism, and since each device's events ` +
				`still hash to a single partition, ordering is preserved.` +
				"\n\n" +
				`**The Partition Increase Problem**` +
				"\n\n" +
				`When you increase partitions, existing keys may map to different partitions. For a brief ` +
				`period, events for the same device could exist in two different partitions — the old one ` +
				`(with historical data) and the new one (with new data). This doesn't break ordering for ` +
				`new data, but it means you can't replay the entire topic and get perfect per-device ordering.` +
				"\n\n" +
				`Solutions for the partition increase:` +
				"\n" +
				`1. **Coordinate the increase during low traffic**: Drain existing messages first, increase ` +
				`partitions, then resume production. This gives you a clean cutover.` +
				"\n" +
				`2. **Use a custom partitioner**: Implement a partitioner that uses consistent hashing ` +
				`(e.g., jump consistent hash). This minimizes key redistribution when partitions change — ` +
				`only keys that map to the new partitions move.` +
				"\n" +
				`3. **Over-provision from the start**: Set partitions to your expected maximum (e.g., 96 ` +
				`or 128) even if you only have 12 consumers now. Unused partitions have minimal overhead. ` +
				`This avoids ever needing to repartition.` +
				"\n\n" +
				`**Recommended Partition Count**` +
				"\n\n" +
				`For 500k events/sec with growth headroom, I'd recommend 48-64 partitions:` +
				"\n" +
				`- Gives 4-5x headroom over current throughput` +
				"\n" +
				`- Each partition handles ~8-10k events/sec, well within Kafka's per-partition capacity` +
				"\n" +
				`- With RF=3, you'll have 144-192 replicas across your brokers — ensure you have ` +
				`sufficient broker capacity (typically 5+ brokers for this setup)` +
				"\n" +
				`- Consumer-side parallelism scales linearly: you can run 48-64 consumers` +
				"\n\n" +
				`**Alternative: Partition-Local Ordering with External Sequencing**` +
				"\n\n" +
				`If your ordering requirements are strict (you need to detect out-of-order events), ` +
				`include a per-device monotonic sequence number in your events. The consumer can then ` +
				`detect and handle ordering violations regardless of partition layout:` +
				"\n\n" +
				"```go" +
				"\n" +
				`type Event struct {` +
				"\n" +
				`    DeviceID  string    ` + "`json:\"device_id\"`" +
				"\n" +
				`    SeqNum    uint64    ` + "`json:\"seq_num\"`" +
				"\n" +
				`    Timestamp time.Time ` + "`json:\"timestamp\"`" +
				"\n" +
				`    Payload   []byte    ` + "`json:\"payload\"`" +
				"\n" +
				`}` +
				"\n" +
				"```" +
				"\n\n" +
				`This decouples your ordering guarantee from Kafka's partition topology and makes ` +
				`your system resilient to any future partition changes.` +
				"\n\n" +
				`My recommendation: increase to 64 partitions using option 1 (coordinated during ` +
				`low traffic) and add sequence numbers for defense in depth. The partition increase ` +
				`is a one-time operation and the sequence numbers give you a permanent safety net.`,
		},
		{
			question: `Our Redis caching layer is another area of concern. We're using Redis 7 as a ` +
				`look-aside cache for device metadata and configuration. Currently we have about 2 million ` +
				`keys consuming 12GB of memory. We're seeing cache hit rates around 85% but during ` +
				`deployments the hit rate drops to 40% for several minutes causing a thundering herd ` +
				`on PostgreSQL. How should we handle cache warming and invalidation?`,
			answer: `An 85% baseline hit rate that drops to 40% during deployments is a serious operational ` +
				`concern, especially at your throughput. The thundering herd effect can cascade into ` +
				`PostgreSQL connection exhaustion, which would take down the entire pipeline. Let's ` +
				`address both the immediate deployment problem and the general caching architecture.` +
				"\n\n" +
				`**Why Deployments Kill Your Cache**` +
				"\n\n" +
				`The hit rate drop during deployment suggests one of these scenarios:` +
				"\n" +
				`1. Your cache keys include a version identifier or deployment hash (common mistake)` +
				"\n" +
				`2. You're flushing Redis during deployment (never do this in production)` +
				"\n" +
				`3. New pods start with cold local caches and overwhelm Redis and PostgreSQL` +
				"\n" +
				`4. Connection pool reset during rolling restart causes temporary cache bypass` +
				"\n\n" +
				`The most likely cause is #3 or #4. During a rolling deployment, Kubernetes terminates ` +
				`old pods and starts new ones. Each new pod establishes fresh Redis connections and ` +
				`any in-process caches (sync.Map, local LRU) are empty.` +
				"\n\n" +
				`**Solving the Thundering Herd**` +
				"\n\n" +
				`Layer 1 — Single-flight protection (critical, implement immediately):` +
				"\n\n" +
				"```go" +
				"\n" +
				`import "golang.org/x/sync/singleflight"` +
				"\n\n" +
				`var deviceGroup singleflight.Group` +
				"\n\n" +
				`func GetDeviceConfig(ctx context.Context, deviceID string) (*DeviceConfig, error) {` +
				"\n" +
				`    val, err, _ := deviceGroup.Do(deviceID, func() (any, error) {` +
				"\n" +
				`        // Check Redis first` +
				"\n" +
				`        cfg, err := redis.Get(ctx, "device:"+deviceID)` +
				"\n" +
				`        if err == nil {` +
				"\n" +
				`            return cfg, nil` +
				"\n" +
				`        }` +
				"\n" +
				`        // Cache miss: load from PostgreSQL` +
				"\n" +
				`        cfg, err = db.LoadDeviceConfig(ctx, deviceID)` +
				"\n" +
				`        if err != nil {` +
				"\n" +
				`            return nil, err` +
				"\n" +
				`        }` +
				"\n" +
				`        // Populate cache with TTL` +
				"\n" +
				`        redis.Set(ctx, "device:"+deviceID, cfg, 30*time.Minute)` +
				"\n" +
				`        return cfg, nil` +
				"\n" +
				`    })` +
				"\n" +
				`    if err != nil {` +
				"\n" +
				`        return nil, err` +
				"\n" +
				`    }` +
				"\n" +
				`    return val.(*DeviceConfig), nil` +
				"\n" +
				`}` +
				"\n" +
				"```" +
				"\n\n" +
				`This ensures that if 1000 goroutines request the same device concurrently (thundering ` +
				`herd), only ONE actually hits PostgreSQL. The rest wait and share the result. This ` +
				`alone can reduce your PostgreSQL load during cache misses by 100-1000x.` +
				"\n\n" +
				`Layer 2 — Stale-while-revalidate (background refresh):` +
				"\n" +
				`Instead of hard TTL expiration, serve stale data while refreshing in the background:` +
				"\n" +
				`- Store values with two timestamps: created_at and stale_after` +
				"\n" +
				`- When a value is past stale_after but not expired, return it immediately AND trigger ` +
				`a background refresh` +
				"\n" +
				`- Set the hard expiry at 2x the stale time as a safety net` +
				"\n" +
				`- This eliminates all user-facing cache miss latency except for truly new keys` +
				"\n\n" +
				`Layer 3 — Cache warming during deployment:` +
				"\n" +
				`Add a readiness probe that pre-warms critical cache entries before the pod accepts traffic:` +
				"\n" +
				`- On startup, load the top 10,000 most-accessed device configs from PostgreSQL into Redis` +
				"\n" +
				`- Use the access frequency data from Redis itself (use Redis OBJECT FREQ or maintain ` +
				`your own access counter)` +
				"\n" +
				`- The readiness probe should not return healthy until warming is complete` +
				"\n" +
				`- This ensures new pods never face a completely cold cache` +
				"\n\n" +
				`**Cache Invalidation Strategy**` +
				"\n\n" +
				`For device configuration, use event-driven invalidation rather than TTL-only:` +
				"\n" +
				`- Publish a Kafka event when device config changes` +
				"\n" +
				`- All service instances subscribe and invalidate the specific key` +
				"\n" +
				`- Keep a short TTL (30 min) as a backstop for missed events` +
				"\n" +
				`- This gives you near-instant cache coherence without polling` +
				"\n\n" +
				`**Memory Optimization**` +
				"\n\n" +
				`12GB for 2M keys means ~6KB per key on average. For device metadata, this seems high. ` +
				`Consider:` +
				"\n" +
				`- Use Redis Hash type instead of String for structured data (better memory efficiency)` +
				"\n" +
				`- Enable hash-max-ziplist-entries and hash-max-ziplist-value for compact storage` +
				"\n" +
				`- Use MessagePack instead of JSON for serialization (typically 30-40% smaller)` +
				"\n" +
				`- Consider Redis 7's listpack encoding which further improves memory efficiency` +
				"\n\n" +
				`The combination of singleflight + stale-while-revalidate + deployment warming should ` +
				`eliminate your thundering herd problem entirely and maintain 95%+ hit rates even ` +
				`during deployments.`,
		},
		{
			question: `Let's discuss our observability stack. We're using OpenTelemetry for tracing and ` +
				`Prometheus for metrics, but our engineers are struggling to correlate traces with ` +
				`metrics during incidents. We also have too many alerts firing — about 40% of our ` +
				`alerts are false positives that get ignored. How should we restructure our ` +
				`observability approach?`,
			answer: `Alert fatigue (40% false positives) combined with poor trace-metric correlation is ` +
				`a classic sign of observability that was built bottom-up (instrument everything) ` +
				`rather than top-down (what questions do we need to answer?). Let me propose a ` +
				`restructured approach.` +
				"\n\n" +
				`**The USE/RED Framework**` +
				"\n\n" +
				`Start by organizing your metrics around two proven frameworks:` +
				"\n" +
				`- **USE** (Utilization, Saturation, Errors) for infrastructure: CPU utilization, ` +
				`memory saturation, disk I/O errors` +
				"\n" +
				`- **RED** (Rate, Errors, Duration) for services: request rate, error rate, request ` +
				`duration percentiles` +
				"\n\n" +
				`Every metric should answer one of these six questions. If a metric doesn't fit either ` +
				`framework, question whether you need it. For your platform, the key RED metrics per ` +
				`service would be:` +
				"\n\n" +
				"```" +
				"\n" +
				`# Rate: events processed per second` +
				"\n" +
				`kronk_events_processed_total{service, status}` +
				"\n\n" +
				`# Errors: failed events per second` +
				"\n" +
				`kronk_events_errors_total{service, error_type}` +
				"\n\n" +
				`# Duration: processing latency histogram` +
				"\n" +
				`kronk_event_processing_duration_seconds{service, quantile}` +
				"\n" +
				"```" +
				"\n\n" +
				`**Fixing Alert Fatigue**` +
				"\n\n" +
				`Your 40% false positive rate suggests your alerts are based on symptoms rather than ` +
				`impact. Restructure using this hierarchy:` +
				"\n\n" +
				`1. **Page-worthy alerts** (wake someone up): Only SLO violations` +
				"\n" +
				`   - Error budget burn rate exceeds 14.4x (will exhaust monthly budget in 1 hour)` +
				"\n" +
				`   - Error budget burn rate exceeds 6x sustained for 5 minutes` +
				"\n" +
				`   - Data pipeline stopped processing (zero throughput for 2+ minutes)` +
				"\n\n" +
				`2. **Ticket-worthy alerts** (fix during business hours): Degradation trending` +
				"\n" +
				`   - Error budget burn rate exceeds 1x sustained for 6 hours` +
				"\n" +
				`   - Consumer lag growing continuously for 15+ minutes` +
				"\n" +
				`   - Cache hit rate below 70% for 10+ minutes` +
				"\n" +
				`   - Database connection pool above 80% capacity for 10+ minutes` +
				"\n\n" +
				`3. **Dashboard-only** (no alert): Everything else` +
				"\n" +
				`   - Per-pod CPU/memory metrics` +
				"\n" +
				`   - Individual node health` +
				"\n" +
				`   - GC pause times (useful for debugging but not alerting)` +
				"\n\n" +
				`The key principle: alert on customer-facing impact and error budget consumption, not ` +
				`on internal symptoms. A single pod restarting is not page-worthy if the service is ` +
				`still meeting its SLO.` +
				"\n\n" +
				`**Correlating Traces with Metrics**` +
				"\n\n" +
				`The correlation gap usually comes from missing the link between a metric spike and ` +
				`the specific requests causing it. Bridge this with:` +
				"\n\n" +
				`1. **Exemplars**: OpenTelemetry exemplars attach trace IDs to metric samples. When ` +
				`you see a latency spike in Grafana, you can click through to the exact trace:` +
				"\n\n" +
				"```go" +
				"\n" +
				`histogram.Record(ctx, duration,` +
				"\n" +
				`    metric.WithAttributes(` +
				"\n" +
				`        attribute.String("service", "event-processor"),` +
				"\n" +
				`    ),` +
				"\n" +
				`)` +
				"\n" +
				"```" +
				"\n\n" +
				`Grafana 10+ natively supports exemplar display on histograms, linking directly ` +
				`to Tempo or Jaeger traces.` +
				"\n\n" +
				`2. **Structured logging with trace context**: Ensure every log line includes ` +
				`trace_id and span_id. Use the OpenTelemetry log bridge so logs, traces, and ` +
				`metrics all share the same context:` +
				"\n\n" +
				"```go" +
				"\n" +
				`logger.InfoContext(ctx, "event processed",` +
				"\n" +
				`    slog.Int("device_count", count),` +
				"\n" +
				`    slog.Duration("duration", elapsed),` +
				"\n" +
				`)` +
				"\n" +
				"```" +
				"\n\n" +
				`3. **Service-level dashboards**: Build per-service dashboards following a standard ` +
				`template. Each dashboard should have panels for RED metrics, a trace search panel, ` +
				`and recent error logs. This gives engineers a single landing page for any service ` +
				`investigation.` +
				"\n\n" +
				`**Practical Implementation Order**` +
				"\n\n" +
				`Week 1: Implement SLO-based alerting and delete all symptom-based alerts` +
				"\n" +
				`Week 2: Add exemplars to your key latency histograms` +
				"\n" +
				`Week 3: Standardize structured logging with trace context` +
				"\n" +
				`Week 4: Build service-level dashboard templates` +
				"\n\n" +
				`This restructuring typically reduces alert volume by 70-80% while improving ` +
				`incident detection speed because every alert that fires is actionable.`,
		},
	}
}

// =============================================================================
// Benchmark harness

type benchResult struct {
	ttft         time.Duration
	totalTime    time.Duration
	tps          float64
	promptTokens int
	outputTokens int
}

// runStreamingBench executes a single streaming chat request and captures
// TTFT, TPS, total wall-clock time, and token counts.
func runStreamingBench(ctx context.Context, krn *kronk.Kronk, d model.D) (benchResult, error) {
	start := time.Now()

	ch, err := krn.ChatStreaming(ctx, d)
	if err != nil {
		return benchResult{}, fmt.Errorf("chat streaming: %w", err)
	}

	var (
		result    benchResult
		ttftSet   bool
		lastResp  model.ChatResponse
		respError error
	)

	for resp := range ch {
		if !ttftSet {
			if len(resp.Choice) > 0 {
				delta := resp.Choice[0].Delta
				if delta != nil && (delta.Content != "" || delta.Reasoning != "") {
					result.ttft = time.Since(start)
					ttftSet = true
				}
			}
		}

		// Check for error responses.
		if len(resp.Choice) > 0 && resp.Choice[0].FinishReason() == "error" {
			if resp.Choice[0].Message != nil {
				respError = fmt.Errorf("model error: %s", resp.Choice[0].Message.Content)
			}
		}

		lastResp = resp
	}

	result.totalTime = time.Since(start)

	if respError != nil {
		return benchResult{}, respError
	}

	if lastResp.Usage != nil {
		result.tps = lastResp.Usage.TokensPerSecond
		result.promptTokens = lastResp.Usage.PromptTokens
		result.outputTokens = lastResp.Usage.OutputTokens
	}

	return result, nil
}

// logTokenCounts uses the model's tokenizer to log actual token counts for
// the system prompt and total conversation. This provides exact numbers
// rather than estimates based on chars-per-token ratios.
func logTokenCounts(b *testing.B, ctx context.Context, krn *kronk.Kronk, d model.D) {
	b.Helper()

	messages, _ := d["messages"].([]model.D)
	if len(messages) == 0 {
		return
	}

	// Tokenize the system prompt.
	if content, _ := messages[0]["content"].(string); content != "" {
		if role, _ := messages[0]["role"].(string); role == "system" {
			resp, err := krn.Tokenize(ctx, model.D{"input": content})
			if err != nil {
				b.Logf("tokenize system prompt: %v", err)
			} else {
				b.Logf("System prompt: %d tokens (%d chars)", resp.Tokens, len(content))
			}
		}
	}

	// Tokenize all message content concatenated for total count.
	var totalChars int
	var allContent string
	for _, msg := range messages {
		if content, _ := msg["content"].(string); content != "" {
			allContent += content
			totalChars += len(content)
		}
	}

	resp, err := krn.Tokenize(ctx, model.D{"input": allContent})
	if err != nil {
		b.Logf("tokenize conversation: %v", err)
	} else {
		b.Logf("Total conversation: %d tokens (%d chars, %d messages)", resp.Tokens, totalChars, len(messages))
	}
}

// withBenchModel loads a model for benchmarking and handles cleanup.
func withBenchModel(b *testing.B, cfg model.Config) *kronk.Kronk {
	b.Helper()

	krn, err := kronk.New(cfg)
	if err != nil {
		b.Fatalf("unable to load model: %v", err)
	}

	b.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			b.Errorf("failed to unload model: %v", err)
		}
	})

	return krn
}

// benchChat runs the core benchmark loop for a given mode and document.
func benchChat(b *testing.B, krn *kronk.Kronk, d model.D) {
	b.Helper()
	b.ReportAllocs()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Log actual token counts for the system prompt and full conversation.
	logTokenCounts(b, ctx, krn, d)

	// Warmup: prime caches and JIT paths.

	if _, err := runStreamingBench(ctx, krn, d); err != nil {
		b.Fatalf("warmup failed: %v", err)
	}

	b.ResetTimer()

	for b.Loop() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)

		result, err := runStreamingBench(ctx, krn, d)
		cancel()

		if err != nil {
			b.Fatalf("benchmark iteration failed: %v", err)
		}

		b.ReportMetric(float64(result.ttft.Milliseconds()), "ttft-ms")
		b.ReportMetric(result.tps, "tok/s")
		b.ReportMetric(float64(result.totalTime.Milliseconds()), "total-ms")
		b.ReportMetric(float64(result.promptTokens), "prompt-tok")
		b.ReportMetric(float64(result.outputTokens), "output-tok")
	}
}

// =============================================================================
// Benchmarks: Non-Caching

func BenchmarkNonCaching(b *testing.B) {
	krn := withBenchModel(b, cfgNonCaching())
	benchChat(b, krn, benchDoc())
}

// =============================================================================
// Benchmarks: System Prompt Cache (SPC)

func BenchmarkSPC(b *testing.B) {
	krn := withBenchModel(b, cfgSPC())
	benchChat(b, krn, benchDoc())
}

// =============================================================================
// Benchmarks: Incremental Message Cache (IMC)

func BenchmarkIMC(b *testing.B) {
	krn := withBenchModel(b, cfgIMC())
	benchChat(b, krn, benchDoc())
}
