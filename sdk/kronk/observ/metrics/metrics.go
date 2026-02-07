// Package metrics constructs the metrics the application will track.
package metrics

import (
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type agg struct {
	sum   float64
	count float64
	min   float64
	max   float64
}

func newAgg() agg {
	return agg{min: math.MaxFloat64}
}

type aggSnapshot struct {
	avg float64
	min float64
	max float64
}

func observe(a *agg, v float64) aggSnapshot {
	a.sum += v
	a.count++

	if v < a.min {
		a.min = v
	}

	if v > a.max {
		a.max = v
	}

	return aggSnapshot{
		avg: a.sum / a.count,
		min: a.min,
		max: a.max,
	}
}

type modelState struct {
	modelLoad        agg
	modelLoadProj    agg
	promptCreation   agg
	prefill          agg
	ttft             agg
	promptTokens     agg
	reasoningTokens  agg
	completionTokens agg
	outputTokens     agg
	totalTokens      agg
	tokensPerSecond  agg
}

func newModelState() *modelState {
	return &modelState{
		modelLoad:        newAgg(),
		modelLoadProj:    newAgg(),
		promptCreation:   newAgg(),
		prefill:          newAgg(),
		ttft:             newAgg(),
		promptTokens:     newAgg(),
		reasoningTokens:  newAgg(),
		completionTokens: newAgg(),
		outputTokens:     newAgg(),
		totalTokens:      newAgg(),
		tokensPerSecond:  newAgg(),
	}
}

var (
	m      promMetrics
	mu     sync.Mutex
	states = map[string]*modelState{}
)

func stateFor(modelID string) *modelState {
	if modelID == "" {
		modelID = "unknown"
	}

	s := states[modelID]
	if s == nil {
		s = newModelState()
		states[modelID] = s
	}

	return s
}

type promMetrics struct {
	goroutines prometheus.Gauge
	requests   prometheus.Counter
	errors     prometheus.Counter
	panics     prometheus.Counter

	modelLoadAvg *prometheus.GaugeVec
	modelLoadMin *prometheus.GaugeVec
	modelLoadMax *prometheus.GaugeVec

	modelLoadProjAvg *prometheus.GaugeVec
	modelLoadProjMin *prometheus.GaugeVec
	modelLoadProjMax *prometheus.GaugeVec

	promptCreationAvg *prometheus.GaugeVec
	promptCreationMin *prometheus.GaugeVec
	promptCreationMax *prometheus.GaugeVec

	prefillAvg *prometheus.GaugeVec
	prefillMin *prometheus.GaugeVec
	prefillMax *prometheus.GaugeVec

	ttftAvg *prometheus.GaugeVec
	ttftMin *prometheus.GaugeVec
	ttftMax *prometheus.GaugeVec

	promptTokensAvg *prometheus.GaugeVec
	promptTokensMin *prometheus.GaugeVec
	promptTokensMax *prometheus.GaugeVec

	reasoningTokensAvg *prometheus.GaugeVec
	reasoningTokensMin *prometheus.GaugeVec
	reasoningTokensMax *prometheus.GaugeVec

	completionTokensAvg *prometheus.GaugeVec
	completionTokensMin *prometheus.GaugeVec
	completionTokensMax *prometheus.GaugeVec

	outputTokensAvg *prometheus.GaugeVec
	outputTokensMin *prometheus.GaugeVec
	outputTokensMax *prometheus.GaugeVec

	totalTokensAvg *prometheus.GaugeVec
	totalTokensMin *prometheus.GaugeVec
	totalTokensMax *prometheus.GaugeVec

	tokensPerSecondAvg *prometheus.GaugeVec
	tokensPerSecondMin *prometheus.GaugeVec
	tokensPerSecondMax *prometheus.GaugeVec

	vramTotal  *prometheus.GaugeVec
	slotMemory *prometheus.GaugeVec
}

func init() {
	m = promMetrics{
		goroutines: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "goroutines",
			Help: "Number of goroutines",
		}),
		requests: promauto.NewCounter(prometheus.CounterOpts{
			Name: "requests",
			Help: "Total number of requests",
		}),
		errors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "errors",
			Help: "Total number of errors",
		}),
		panics: promauto.NewCounter(prometheus.CounterOpts{
			Name: "panics",
			Help: "Total number of panics",
		}),

		modelLoadAvg: newGaugeVec("model_load_avg", "Model load time average in seconds"),
		modelLoadMin: newGaugeVec("model_load_min", "Model load time minimum in seconds"),
		modelLoadMax: newGaugeVec("model_load_max", "Model load time maximum in seconds"),

		modelLoadProjAvg: newGaugeVec("model_load_proj_avg", "Proj file load time average in seconds"),
		modelLoadProjMin: newGaugeVec("model_load_proj_min", "Proj file load time minimum in seconds"),
		modelLoadProjMax: newGaugeVec("model_load_proj_max", "Proj file load time maximum in seconds"),

		promptCreationAvg: newGaugeVec("model_prompt_creation_avg", "Prompt creation time average in seconds"),
		promptCreationMin: newGaugeVec("model_prompt_creation_min", "Prompt creation time minimum in seconds"),
		promptCreationMax: newGaugeVec("model_prompt_creation_max", "Prompt creation time maximum in seconds"),

		prefillAvg: newGaugeVec("model_prefill_avg", "Prefill time average in seconds"),
		prefillMin: newGaugeVec("model_prefill_min", "Prefill time minimum in seconds"),
		prefillMax: newGaugeVec("model_prefill_max", "Prefill time maximum in seconds"),

		ttftAvg: newGaugeVec("model_ttft_avg", "Time to first token average in seconds"),
		ttftMin: newGaugeVec("model_ttft_min", "Time to first token minimum in seconds"),
		ttftMax: newGaugeVec("model_ttft_max", "Time to first token maximum in seconds"),

		promptTokensAvg: newGaugeVec("usage_prompt_tokens_avg", "Prompt tokens average"),
		promptTokensMin: newGaugeVec("usage_prompt_tokens_min", "Prompt tokens minimum"),
		promptTokensMax: newGaugeVec("usage_prompt_tokens_max", "Prompt tokens maximum"),

		reasoningTokensAvg: newGaugeVec("usage_reasoning_tokens_avg", "Reasoning tokens average"),
		reasoningTokensMin: newGaugeVec("usage_reasoning_tokens_min", "Reasoning tokens minimum"),
		reasoningTokensMax: newGaugeVec("usage_reasoning_tokens_max", "Reasoning tokens maximum"),

		completionTokensAvg: newGaugeVec("usage_completion_tokens_avg", "Completion tokens average"),
		completionTokensMin: newGaugeVec("usage_completion_tokens_min", "Completion tokens minimum"),
		completionTokensMax: newGaugeVec("usage_completion_tokens_max", "Completion tokens maximum"),

		outputTokensAvg: newGaugeVec("usage_output_tokens_avg", "Output tokens average"),
		outputTokensMin: newGaugeVec("usage_output_tokens_min", "Output tokens minimum"),
		outputTokensMax: newGaugeVec("usage_output_tokens_max", "Output tokens maximum"),

		totalTokensAvg: newGaugeVec("usage_total_tokens_avg", "Total tokens average"),
		totalTokensMin: newGaugeVec("usage_total_tokens_min", "Total tokens minimum"),
		totalTokensMax: newGaugeVec("usage_total_tokens_max", "Total tokens maximum"),

		tokensPerSecondAvg: newGaugeVec("usage_tokens_per_second_avg", "Tokens per second average"),
		tokensPerSecondMin: newGaugeVec("usage_tokens_per_second_min", "Tokens per second minimum"),
		tokensPerSecondMax: newGaugeVec("usage_tokens_per_second_max", "Tokens per second maximum"),

		vramTotal:  newGaugeVec("vram_total_bytes", "Total estimated VRAM usage in bytes (model weights + KV cache)"),
		slotMemory: newGaugeVec("vram_slot_memory_bytes", "KV cache slot memory in bytes"),
	}
}

func newGaugeVec(name, help string) *prometheus.GaugeVec {
	return promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, []string{"model_id"})
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
	secs := duration.Seconds()

	mu.Lock()
	s := stateFor(modelID)
	snap := observe(&s.modelLoad, secs)
	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.modelLoadAvg.WithLabelValues(modelID).Set(snap.avg)
	m.modelLoadMin.WithLabelValues(modelID).Set(snap.min)
	m.modelLoadMax.WithLabelValues(modelID).Set(snap.max)
}

// AddProjFileLoadTime captures the specified duration for loading a proj file.
func AddProjFileLoadTime(modelID string, duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	s := stateFor(modelID)
	snap := observe(&s.modelLoadProj, secs)
	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.modelLoadProjAvg.WithLabelValues(modelID).Set(snap.avg)
	m.modelLoadProjMin.WithLabelValues(modelID).Set(snap.min)
	m.modelLoadProjMax.WithLabelValues(modelID).Set(snap.max)
}

// AddPromptCreationTime captures the specified duration for creating a prompt.
func AddPromptCreationTime(modelID string, duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	s := stateFor(modelID)
	snap := observe(&s.promptCreation, secs)
	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.promptCreationAvg.WithLabelValues(modelID).Set(snap.avg)
	m.promptCreationMin.WithLabelValues(modelID).Set(snap.min)
	m.promptCreationMax.WithLabelValues(modelID).Set(snap.max)
}

// AddPrefillTime captures the specified duration for prefilling a request.
func AddPrefillTime(modelID string, duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	s := stateFor(modelID)
	snap := observe(&s.prefill, secs)
	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.prefillAvg.WithLabelValues(modelID).Set(snap.avg)
	m.prefillMin.WithLabelValues(modelID).Set(snap.min)
	m.prefillMax.WithLabelValues(modelID).Set(snap.max)
}

// AddTimeToFirstToken captures the specified duration for ttft.
func AddTimeToFirstToken(modelID string, duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	s := stateFor(modelID)
	snap := observe(&s.ttft, secs)
	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.ttftAvg.WithLabelValues(modelID).Set(snap.avg)
	m.ttftMin.WithLabelValues(modelID).Set(snap.min)
	m.ttftMax.WithLabelValues(modelID).Set(snap.max)
}

// AddChatCompletionsUsage captures the specified usage values for chat-completions.
func AddChatCompletionsUsage(modelID string, promptTokens, reasoningTokens, completionTokens, outputTokens, totalTokens int, tokensPerSecond float64) {
	mu.Lock()
	s := stateFor(modelID)

	ptSnap := observe(&s.promptTokens, float64(promptTokens))
	rtSnap := observe(&s.reasoningTokens, float64(reasoningTokens))
	ctSnap := observe(&s.completionTokens, float64(completionTokens))
	otSnap := observe(&s.outputTokens, float64(outputTokens))
	ttSnap := observe(&s.totalTokens, float64(totalTokens))
	tpsSnap := observe(&s.tokensPerSecond, tokensPerSecond)

	mu.Unlock()

	if modelID == "" {
		modelID = "unknown"
	}

	m.promptTokensAvg.WithLabelValues(modelID).Set(ptSnap.avg)
	m.promptTokensMin.WithLabelValues(modelID).Set(ptSnap.min)
	m.promptTokensMax.WithLabelValues(modelID).Set(ptSnap.max)

	m.reasoningTokensAvg.WithLabelValues(modelID).Set(rtSnap.avg)
	m.reasoningTokensMin.WithLabelValues(modelID).Set(rtSnap.min)
	m.reasoningTokensMax.WithLabelValues(modelID).Set(rtSnap.max)

	m.completionTokensAvg.WithLabelValues(modelID).Set(ctSnap.avg)
	m.completionTokensMin.WithLabelValues(modelID).Set(ctSnap.min)
	m.completionTokensMax.WithLabelValues(modelID).Set(ctSnap.max)

	m.outputTokensAvg.WithLabelValues(modelID).Set(otSnap.avg)
	m.outputTokensMin.WithLabelValues(modelID).Set(otSnap.min)
	m.outputTokensMax.WithLabelValues(modelID).Set(otSnap.max)

	m.totalTokensAvg.WithLabelValues(modelID).Set(ttSnap.avg)
	m.totalTokensMin.WithLabelValues(modelID).Set(ttSnap.min)
	m.totalTokensMax.WithLabelValues(modelID).Set(ttSnap.max)

	m.tokensPerSecondAvg.WithLabelValues(modelID).Set(tpsSnap.avg)
	m.tokensPerSecondMin.WithLabelValues(modelID).Set(tpsSnap.min)
	m.tokensPerSecondMax.WithLabelValues(modelID).Set(tpsSnap.max)
}

func SetVRAM(modelID string, vramTotal, slotMemory int64) {
	if modelID == "" {
		modelID = "unknown"
	}

	m.vramTotal.WithLabelValues(modelID).Set(float64(vramTotal))
	m.slotMemory.WithLabelValues(modelID).Set(float64(slotMemory))
}

func ClearVRAM(modelID string) {
	if modelID == "" {
		modelID = "unknown"
	}

	m.vramTotal.DeleteLabelValues(modelID)
	m.slotMemory.DeleteLabelValues(modelID)
}
