// Package metrics constructs the metrics the application will track.
//
// All timing distributions are exposed as Prometheus histograms (suffix
// "_seconds") so dashboards and PromQL queries can compute rate, average,
// and quantile over arbitrary time windows. Token counts are exposed as
// counters (suffix "_total") so they can be rated and summed across the
// fleet. Per-request tokens-per-second is a histogram of observations so
// it can be averaged or quantiled like any other timing.
//
// PromQL recipes:
//
//	# Rolling average of model load time, per model.
//	rate(model_load_seconds_sum[5m]) / rate(model_load_seconds_count[5m])
//
//	# 99th-percentile end-to-end TTFT.
//	histogram_quantile(0.99, sum by (le, model_id) (rate(model_request_ttft_seconds_bucket[5m])))
//
//	# Total tokens served per second by kind.
//	sum by (kind) (rate(usage_tokens_total[1m]))
package metrics

import (
	"runtime"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// reg is the private Prometheus registry that owns every Kronk collector.
//
// We deliberately do NOT register against prometheus.DefaultRegisterer
// because this package is part of the SDK and may be imported by callers
// who use the SDK directly (no KMS). The default registry is a global
// owned by the application, and a library that writes to it would:
//
//   - silently expose Kronk's series on the caller's /metrics endpoint,
//   - and panic the caller's process if any of Kronk's metric names
//     collided with one they registered themselves (e.g. "requests",
//     "errors", "goroutines").
//
// All Kronk metrics live on this private registry instead. KMS exposes
// it via the /metrics handler (see cmd/server/app/sdk/debug); SDK-only
// callers can ignore it, in which case observations are recorded into
// collectors that are never scraped (a few atomic ops, effectively free).
//
// Callers who want to merge Kronk's metrics into their own registry can
// use Gatherer() with prometheus.Gatherers{} or wrap it as a collector.
var reg = prometheus.NewRegistry()

// auto is a promauto Factory bound to the private registry, so every
// auto.NewXxx call below registers against reg instead of the
// global default registerer.
var auto = promauto.With(reg)

// Gatherer returns the Prometheus gatherer that holds Kronk's metrics.
// The KMS HTTP /metrics handler exposes this. SDK-only callers can
// ignore it, or merge it into their own registry via prometheus.Gatherers.
func Gatherer() prometheus.Gatherer {
	return reg
}

// Histogram bucket families. We pick families that match the expected
// magnitude of each measurement — a too-narrow set of buckets gives
// useless quantiles, and overly wide buckets waste cardinality.
var (
	// loadBuckets covers model and proj-file load times, which range from
	// a few seconds for small quantized weights to several minutes for
	// large MoE checkpoints on cold disks.
	loadBuckets = []float64{1, 2, 5, 10, 20, 30, 60, 120, 300, 600}

	// subSecondBuckets is appropriate for prompt creation, prefill
	// chunks, and engine-internal TTFT — all expected to be sub-second
	// in normal operation. Uses the Prometheus default family.
	subSecondBuckets = prometheus.DefBuckets

	// requestTTFTBuckets covers user-perceived TTFT, which includes
	// queue wait and can stretch into the tens of seconds under load.
	requestTTFTBuckets = []float64{0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120}

	// tpsBuckets covers per-request decode rates from very small models
	// (~5 tps on CPU) to small quantized models on fast GPUs (~1000 tps).
	tpsBuckets = []float64{1, 5, 10, 25, 50, 100, 250, 500, 1000}
)

type promMetrics struct {
	goroutines prometheus.Gauge
	requests   prometheus.Counter
	errors     prometheus.Counter
	panics     prometheus.Counter

	modelLoadSeconds      *prometheus.HistogramVec // labels: model_id.
	modelLoadProjSeconds  *prometheus.HistogramVec // labels: model_id.
	promptCreationSeconds *prometheus.HistogramVec // labels: model_id.
	prefillSeconds        *prometheus.HistogramVec // labels: model_id, kind.
	prefillTTFTSeconds    *prometheus.HistogramVec // labels: model_id.
	requestTTFTSeconds    *prometheus.HistogramVec // labels: model_id.

	tokensTotal     *prometheus.CounterVec   // labels: model_id, kind.
	tokensPerSecond *prometheus.HistogramVec // labels: model_id.

	vramTotal  *prometheus.GaugeVec // labels: model_id.
	slotMemory *prometheus.GaugeVec // labels: model_id.

	// -------------------------------------------------------------------------
	// Pool metrics (sdk/pool).

	poolAcquireTotal         *prometheus.CounterVec // labels: result=hit|miss|dedup|busy|error.
	poolEvictionsTotal       *prometheus.CounterVec // labels: reason, selection.
	poolEvictBeforeLoadTotal prometheus.Counter
	poolLoadFailuresTotal    *prometheus.CounterVec   // labels: stage.
	poolAcquireDuration      *prometheus.HistogramVec // labels: cache=hit|miss.
	poolSingleflightWait     prometheus.Histogram
	poolEvictWaitSeconds     prometheus.Histogram
	poolUnloadDuration       *prometheus.HistogramVec // labels: model_id.
	poolItemsInCache         prometheus.Gauge
	poolMaxItemsInCache      prometheus.Gauge
	poolActiveStreams        *prometheus.GaugeVec // labels: model_id.
	poolInflightLoads        prometheus.Gauge

	// -------------------------------------------------------------------------
	// Resource manager metrics (sdk/pool/resman).

	resmanBudgetPercent     prometheus.Gauge
	resmanHeadroomBytes     prometheus.Gauge
	resmanUnifiedMemory     prometheus.Gauge
	resmanReservations      prometheus.Gauge
	resmanRAMTotalBytes     prometheus.Gauge
	resmanRAMBudgetBytes    prometheus.Gauge
	resmanRAMUsedBytes      prometheus.Gauge
	resmanRAMFreeBytes      prometheus.Gauge
	resmanDeviceTotalBytes  *prometheus.GaugeVec   // labels: device, type.
	resmanDeviceBudgetBytes *prometheus.GaugeVec   // labels: device, type.
	resmanDeviceUsedBytes   *prometheus.GaugeVec   // labels: device, type.
	resmanDeviceFreeBytes   *prometheus.GaugeVec   // labels: device, type.
	resmanReservationBytes  *prometheus.GaugeVec   // labels: model_id, kind=ram|vram, device.
	resmanRejectionsTotal   *prometheus.CounterVec // labels: reason.

	// -------------------------------------------------------------------------
	// Request-level metrics (chat completions).

	chatRequestsTotal    *prometheus.CounterVec   // labels: model_id, status.
	chatErrorsTotal      *prometheus.CounterVec   // labels: model_id, class.
	chatRequestDuration  *prometheus.HistogramVec // labels: model_id.
	chatQueueWaitSeconds *prometheus.HistogramVec // labels: model_id.
}

var m promMetrics

func init() {

	// The default Prometheus registry auto-installs these runtime
	// collectors (process_*, go_*); since we use a private registry, we
	// have to opt back in so KMS dashboards keep showing GC, heap, fds,
	// goroutines, etc.
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m = promMetrics{
		goroutines: auto.NewGauge(prometheus.GaugeOpts{
			Name: "goroutines",
			Help: "Number of goroutines",
		}),
		requests: auto.NewCounter(prometheus.CounterOpts{
			Name: "requests",
			Help: "Total number of requests",
		}),
		errors: auto.NewCounter(prometheus.CounterOpts{
			Name: "errors",
			Help: "Total number of errors",
		}),
		panics: auto.NewCounter(prometheus.CounterOpts{
			Name: "panics",
			Help: "Total number of panics",
		}),

		modelLoadSeconds:      newHistVec("model_load_seconds", "Model file load time in seconds", loadBuckets),
		modelLoadProjSeconds:  newHistVec("model_load_proj_seconds", "Multimodal proj file load time in seconds", loadBuckets),
		promptCreationSeconds: newHistVec("model_prompt_creation_seconds", "Prompt template render time in seconds", subSecondBuckets),
		prefillSeconds:        newHistVec("model_prefill_seconds", "Prefill time in seconds, by kind (text|media|imc-decode)", subSecondBuckets, "kind"),
		prefillTTFTSeconds:    newHistVec("model_prefill_ttft_seconds", "Prefill-only time-to-first-token in seconds (prefill-start to first sampled token)", subSecondBuckets),
		requestTTFTSeconds:    newHistVec("model_request_ttft_seconds", "End-to-end request time-to-first-token in seconds (request-received to first sampled token)", requestTTFTBuckets),

		tokensTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "usage_tokens_total",
			Help: "Total tokens served, by kind (prompt|reasoning|completion).",
		}, []string{"model_id", "kind"}),
		tokensPerSecond: newHistVec("usage_tokens_per_second", "Per-request decode rate in tokens/second (computed after TTFT)", tpsBuckets),

		vramTotal:  newGaugeVec("vram_total_bytes", "Total estimated VRAM usage in bytes (model weights + KV cache)"),
		slotMemory: newGaugeVec("vram_slot_memory_bytes", "KV cache slot memory in bytes"),

		// Pool.
		poolAcquireTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_acquire_total",
			Help: "Total pool acquire attempts, by result (hit|miss|dedup|busy|error).",
		}, []string{"result"}),
		poolEvictionsTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_evictions_total",
			Help: "Total model evictions, by reason and selection mode.",
		}, []string{"reason", "selection"}),
		poolEvictBeforeLoadTotal: auto.NewCounter(prometheus.CounterOpts{
			Name: "pool_evict_before_load_total",
			Help: "Total times the pool had to evict an idle model before admitting a new load.",
		}),
		poolLoadFailuresTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "pool_load_failures_total",
			Help: "Total pool load failures, by stage (plan|reserve|evict|load).",
		}, []string{"stage"}),
		poolAcquireDuration: auto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pool_acquire_duration_seconds",
			Help:    "End-to-end pool acquire latency in seconds, labeled by cache=hit|miss.",
			Buckets: requestTTFTBuckets,
		}, []string{"cache"}),
		poolSingleflightWait: auto.NewHistogram(prometheus.HistogramOpts{
			Name:    "pool_singleflight_wait_seconds",
			Help:    "Time spent waiting for a duplicate in-flight load to finish.",
			Buckets: requestTTFTBuckets,
		}),
		poolEvictWaitSeconds: auto.NewHistogram(prometheus.HistogramOpts{
			Name:    "pool_evict_wait_seconds",
			Help:    "Time spent waiting for an evicted entry to release its reservation.",
			Buckets: subSecondBuckets,
		}),
		poolUnloadDuration: auto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "pool_unload_duration_seconds",
			Help:    "Model unload duration in seconds.",
			Buckets: subSecondBuckets,
		}, []string{"model_id"}),
		poolItemsInCache: auto.NewGauge(prometheus.GaugeOpts{
			Name: "pool_items_in_cache",
			Help: "Number of distinct model entries currently in the pool cache.",
		}),
		poolMaxItemsInCache: auto.NewGauge(prometheus.GaugeOpts{
			Name: "pool_max_items_in_cache",
			Help: "Maximum number of model entries the pool will keep before TTL/cap eviction.",
		}),
		poolActiveStreams: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "pool_active_streams",
			Help: "Active streaming requests per model.",
		}, []string{"model_id"}),
		poolInflightLoads: auto.NewGauge(prometheus.GaugeOpts{
			Name: "pool_inflight_loads",
			Help: "Number of model loads currently in progress (reservation held but not yet in cache).",
		}),

		// Resource manager.
		resmanBudgetPercent: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_budget_percent",
			Help: "Configured percentage of physical memory the resource manager may commit.",
		}),
		resmanHeadroomBytes: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_headroom_bytes",
			Help: "Per-GPU safety margin (bytes) subtracted after BudgetPercent is applied.",
		}),
		resmanUnifiedMemory: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_unified_memory",
			Help: "1 if the system uses a shared GPU/CPU memory pool (Apple Silicon Metal), else 0.",
		}),
		resmanReservations: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_reservations",
			Help: "Active reservations tracked by the resource manager.",
		}),
		resmanRAMTotalBytes: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_ram_total_bytes",
			Help: "Detected system RAM in bytes.",
		}),
		resmanRAMBudgetBytes: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_ram_budget_bytes",
			Help: "RAM budget in bytes (TotalBytes * BudgetPercent).",
		}),
		resmanRAMUsedBytes: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_ram_used_bytes",
			Help: "RAM currently reserved by loaded models.",
		}),
		resmanRAMFreeBytes: auto.NewGauge(prometheus.GaugeOpts{
			Name: "resman_ram_free_bytes",
			Help: "RAM currently available within the budget.",
		}),
		resmanDeviceTotalBytes: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "resman_device_total_bytes",
			Help: "Per-GPU total memory in bytes.",
		}, []string{"device", "type"}),
		resmanDeviceBudgetBytes: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "resman_device_budget_bytes",
			Help: "Per-GPU budget in bytes after BudgetPercent and headroom.",
		}, []string{"device", "type"}),
		resmanDeviceUsedBytes: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "resman_device_used_bytes",
			Help: "Per-GPU bytes currently reserved.",
		}, []string{"device", "type"}),
		resmanDeviceFreeBytes: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "resman_device_free_bytes",
			Help: "Per-GPU bytes currently free within the budget.",
		}, []string{"device", "type"}),
		resmanReservationBytes: auto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "resman_reservation_bytes",
			Help: "Per-reservation bytes, labeled by model_id, kind=ram|vram, and device.",
		}, []string{"model_id", "kind", "device"}),
		resmanRejectionsTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "resman_reserve_rejections_total",
			Help: "Total reservation rejections by reason (no_capacity|unknown_device|invalid_plan|duplicate_key|no_gpus|other).",
		}, []string{"reason"}),

		// Request-level.
		chatRequestsTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "chat_requests_total",
			Help: "Total chat completion requests by model_id and status (ok|error|cancel).",
		}, []string{"model_id", "status"}),
		chatErrorsTotal: auto.NewCounterVec(prometheus.CounterOpts{
			Name: "chat_errors_total",
			Help: "Chat completion errors by model_id and error class.",
		}, []string{"model_id", "class"}),
		chatRequestDuration:  newHistVec("chat_request_duration_seconds", "End-to-end chat request duration in seconds.", requestTTFTBuckets),
		chatQueueWaitSeconds: newHistVec("chat_queue_wait_seconds", "Time spent waiting in the batch engine queue before being assigned a slot.", subSecondBuckets),
	}
}

func newGaugeVec(name, help string, extraLabels ...string) *prometheus.GaugeVec {
	labels := append([]string{"model_id"}, extraLabels...)
	return auto.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, labels)
}

func newHistVec(name, help string, buckets []float64, extraLabels ...string) *prometheus.HistogramVec {
	labels := append([]string{"model_id"}, extraLabels...)
	return auto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)
}

func normalizeModelID(modelID string) string {
	if modelID == "" {
		return "unknown"
	}
	return modelID
}

// UpdateGoroutines refreshes the goroutine metric.
func UpdateGoroutines() int64 {
	g := int64(runtime.NumGoroutine())
	m.goroutines.Set(float64(g))
	return g
}

// AddRequests increments the request metric by 1.
func AddRequests() int64 {
	m.requests.Inc()
	return 0
}

// AddErrors increments the errors metric by 1.
func AddErrors() int64 {
	m.errors.Inc()
	return 0
}

// AddPanics increments the panics metric by 1.
func AddPanics() int64 {
	m.panics.Inc()
	return 0
}

// AddModelFileLoadTime captures the specified duration for loading a model file.
func AddModelFileLoadTime(modelID string, duration time.Duration) {
	m.modelLoadSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(duration.Seconds())
}

// AddProjFileLoadTime captures the specified duration for loading a proj file.
func AddProjFileLoadTime(modelID string, duration time.Duration) {
	m.modelLoadProjSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(duration.Seconds())
}

// AddPromptCreationTime captures the specified duration for creating a prompt.
func AddPromptCreationTime(modelID string, duration time.Duration) {
	m.promptCreationSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(duration.Seconds())
}

// AddPrefillTime captures the specified duration for prefilling a request.
// Kind distinguishes the source of the prefill: "text" for text token
// prefill batches, "media" for vision/audio mtmd prefill, and "imc-decode"
// for incremental message cache decode work that happens at slot start.
func AddPrefillTime(modelID, kind string, duration time.Duration) {
	if kind == "" {
		kind = "unknown"
	}
	m.prefillSeconds.WithLabelValues(normalizeModelID(modelID), kind).Observe(duration.Seconds())
}

// AddPrefillTTFT captures the time from the start of prefill to the first
// sampled token. This excludes queue wait, tokenization, and cache work.
// For an end-to-end view of user-perceived latency, see AddRequestTTFT.
func AddPrefillTTFT(modelID string, duration time.Duration) {
	m.prefillTTFTSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(duration.Seconds())
}

// AddRequestTTFT captures the time from when a request entered the SDK
// (Chat/ChatStreaming entry) to the first sampled token. This is the
// user-perceived TTFT and includes queue wait, tokenization, cache work,
// and prefill.
func AddRequestTTFT(modelID string, duration time.Duration) {
	m.requestTTFTSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(duration.Seconds())
}

// AddChatCompletionsUsage captures token usage for a completed chat request.
//
// Only the primitive token counts (prompt, reasoning, completion) are
// emitted as counters; output and total are linear combinations of those
// and can be derived in PromQL:
//
//	# Output tokens (reasoning + completion).
//	sum by (model_id) (usage_tokens_total{kind=~"reasoning|completion"})
//
//	# Total tokens (prompt + output).
//	sum by (model_id) (usage_tokens_total)
//
// Per-request tokens-per-second is observed in a histogram so dashboards
// can show its average, percentile, or distribution.
func AddChatCompletionsUsage(modelID string, promptTokens, reasoningTokens, completionTokens, _, _ int, tokensPerSecond float64) {
	id := normalizeModelID(modelID)

	if promptTokens > 0 {
		m.tokensTotal.WithLabelValues(id, "prompt").Add(float64(promptTokens))
	}
	if reasoningTokens > 0 {
		m.tokensTotal.WithLabelValues(id, "reasoning").Add(float64(reasoningTokens))
	}
	if completionTokens > 0 {
		m.tokensTotal.WithLabelValues(id, "completion").Add(float64(completionTokens))
	}

	if tokensPerSecond > 0 {
		m.tokensPerSecond.WithLabelValues(id).Observe(tokensPerSecond)
	}
}

// SetVRAM records the current VRAM footprint of a loaded model.
func SetVRAM(modelID string, vramTotal, slotMemory int64) {
	id := normalizeModelID(modelID)
	m.vramTotal.WithLabelValues(id).Set(float64(vramTotal))
	m.slotMemory.WithLabelValues(id).Set(float64(slotMemory))
}

// ClearVRAM removes the VRAM gauges for a model that has been unloaded.
// Histograms and counters are intentionally left in place so historical
// data is preserved across reload cycles.
func ClearVRAM(modelID string) {
	id := normalizeModelID(modelID)
	m.vramTotal.DeleteLabelValues(id)
	m.slotMemory.DeleteLabelValues(id)
}

// =============================================================================
// Pool helpers.

// AddPoolAcquire increments the pool acquire counter for the given result.
// Result values: "hit" (cache hit), "miss" (loaded fresh), "dedup"
// (singleflight returned an in-flight result), "busy" (ErrServerBusy),
// "error" (other failure).
func AddPoolAcquire(result string) {
	m.poolAcquireTotal.WithLabelValues(result).Inc()
}

// ObservePoolAcquireDuration records the end-to-end duration of an acquire
// call. cacheLabel is "hit" or "miss".
func ObservePoolAcquireDuration(cacheLabel string, d time.Duration) {
	m.poolAcquireDuration.WithLabelValues(cacheLabel).Observe(d.Seconds())
}

// ObservePoolSingleflightWait records time spent waiting for a duplicate
// in-flight load.
func ObservePoolSingleflightWait(d time.Duration) {
	m.poolSingleflightWait.Observe(d.Seconds())
}

// AddPoolEviction increments the eviction counter.
func AddPoolEviction(reason, selection string) {
	if selection == "" {
		selection = "n/a"
	}
	m.poolEvictionsTotal.WithLabelValues(reason, selection).Inc()
}

// AddPoolEvictBeforeLoad increments the evict-before-load counter.
func AddPoolEvictBeforeLoad() {
	m.poolEvictBeforeLoadTotal.Inc()
}

// ObservePoolEvictWait records wait time for an eviction callback to
// release its reservation.
func ObservePoolEvictWait(d time.Duration) {
	m.poolEvictWaitSeconds.Observe(d.Seconds())
}

// AddPoolLoadFailure increments the pool load failure counter.
// Stage values: "plan", "reserve", "evict", "load".
func AddPoolLoadFailure(stage string) {
	m.poolLoadFailuresTotal.WithLabelValues(stage).Inc()
}

// ObservePoolUnloadDuration records the duration of an unload.
func ObservePoolUnloadDuration(modelID string, d time.Duration) {
	m.poolUnloadDuration.WithLabelValues(normalizeModelID(modelID)).Observe(d.Seconds())
}

// SetPoolItemsInCache updates the gauge of items currently in the pool.
func SetPoolItemsInCache(n int) {
	m.poolItemsInCache.Set(float64(n))
}

// SetPoolMaxItemsInCache updates the gauge of the configured cache cap.
func SetPoolMaxItemsInCache(n int) {
	m.poolMaxItemsInCache.Set(float64(n))
}

// SetPoolActiveStreams updates the active streams gauge for a model.
func SetPoolActiveStreams(modelID string, n int) {
	m.poolActiveStreams.WithLabelValues(normalizeModelID(modelID)).Set(float64(n))
}

// ClearPoolActiveStreams removes the active streams gauge for an unloaded model.
func ClearPoolActiveStreams(modelID string) {
	m.poolActiveStreams.DeleteLabelValues(normalizeModelID(modelID))
}

// SetPoolInflightLoads updates the gauge of in-progress loads.
func SetPoolInflightLoads(n int) {
	m.poolInflightLoads.Set(float64(n))
}

// =============================================================================
// Resman helpers.

// ResmanUsage is the subset of resman.Usage that the metrics package needs
// to update its gauges. Defined locally to avoid an import cycle between
// metrics and resman.
type ResmanUsage struct {
	BudgetPercent int
	HeadroomBytes int64
	UnifiedMemory bool
	RAMTotal      int64
	RAMBudget     int64
	RAMUsed       int64
	Devices       []ResmanDeviceUsage
	Reservations  []ResmanReservation
}

// ResmanDeviceUsage describes one device's accounting state.
type ResmanDeviceUsage struct {
	Name        string
	Type        string
	TotalBytes  int64
	BudgetBytes int64
	UsedBytes   int64
}

// ResmanReservation describes one model's reserved memory.
type ResmanReservation struct {
	Key       string
	RAMBytes  int64
	VRAMBytes int64
	Per       []ResmanPerDevice
}

// ResmanPerDevice describes per-device VRAM bytes for a reservation.
type ResmanPerDevice struct {
	Name  string
	Bytes int64
}

// PublishResmanUsage refreshes the resource-manager gauges.
func PublishResmanUsage(u ResmanUsage) {
	m.resmanBudgetPercent.Set(float64(u.BudgetPercent))
	m.resmanHeadroomBytes.Set(float64(u.HeadroomBytes))
	if u.UnifiedMemory {
		m.resmanUnifiedMemory.Set(1)
	} else {
		m.resmanUnifiedMemory.Set(0)
	}
	m.resmanReservations.Set(float64(len(u.Reservations)))

	m.resmanRAMTotalBytes.Set(float64(u.RAMTotal))
	m.resmanRAMBudgetBytes.Set(float64(u.RAMBudget))
	m.resmanRAMUsedBytes.Set(float64(u.RAMUsed))
	free := max(u.RAMBudget-u.RAMUsed, 0)
	m.resmanRAMFreeBytes.Set(float64(free))

	// Reset per-device gauges so devices that disappeared between calls
	// do not show stale values. Reservations are reset for the same reason.
	m.resmanDeviceTotalBytes.Reset()
	m.resmanDeviceBudgetBytes.Reset()
	m.resmanDeviceUsedBytes.Reset()
	m.resmanDeviceFreeBytes.Reset()
	m.resmanReservationBytes.Reset()

	for _, d := range u.Devices {
		m.resmanDeviceTotalBytes.WithLabelValues(d.Name, d.Type).Set(float64(d.TotalBytes))
		m.resmanDeviceBudgetBytes.WithLabelValues(d.Name, d.Type).Set(float64(d.BudgetBytes))
		m.resmanDeviceUsedBytes.WithLabelValues(d.Name, d.Type).Set(float64(d.UsedBytes))
		dFree := max(d.BudgetBytes-d.UsedBytes, 0)
		m.resmanDeviceFreeBytes.WithLabelValues(d.Name, d.Type).Set(float64(dFree))
	}

	for _, r := range u.Reservations {
		id := normalizeModelID(r.Key)
		if r.RAMBytes > 0 {
			m.resmanReservationBytes.WithLabelValues(id, "ram", "host").Set(float64(r.RAMBytes))
		}
		for _, p := range r.Per {
			if p.Bytes > 0 {
				m.resmanReservationBytes.WithLabelValues(id, "vram", p.Name).Set(float64(p.Bytes))
			}
		}
	}
}

// AddResmanRejection increments the resman rejection counter by reason.
func AddResmanRejection(reason string) {
	if reason == "" {
		reason = "other"
	}
	m.resmanRejectionsTotal.WithLabelValues(reason).Inc()
}

// =============================================================================
// Request-level helpers.

// AddChatRequest records the completion of a chat request.
// Status values: "ok", "error", "cancel".
func AddChatRequest(modelID, status string) {
	m.chatRequestsTotal.WithLabelValues(normalizeModelID(modelID), status).Inc()
}

// AddChatError records a chat error by class.
func AddChatError(modelID, class string) {
	if class == "" {
		class = "other"
	}
	m.chatErrorsTotal.WithLabelValues(normalizeModelID(modelID), class).Inc()
}

// ObserveChatRequestDuration records the end-to-end chat request duration.
func ObserveChatRequestDuration(modelID string, d time.Duration) {
	m.chatRequestDuration.WithLabelValues(normalizeModelID(modelID)).Observe(d.Seconds())
}

// ObserveChatQueueWait records the time a request spent in the batch
// engine queue before being assigned to a slot.
func ObserveChatQueueWait(modelID string, d time.Duration) {
	m.chatQueueWaitSeconds.WithLabelValues(normalizeModelID(modelID)).Observe(d.Seconds())
}
