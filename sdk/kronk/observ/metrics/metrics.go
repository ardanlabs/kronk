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

var (
	m  promMetrics
	mu sync.Mutex

	modelLoadSum, modelLoadCount float64
	modelLoadMinVal              float64 = math.MaxFloat64
	modelLoadMaxVal              float64

	modelLoadProjSum, modelLoadProjCount float64
	modelLoadProjMinVal                  float64 = math.MaxFloat64
	modelLoadProjMaxVal                  float64

	promptCreationSum, promptCreationCount float64
	promptCreationMinVal                   float64 = math.MaxFloat64
	promptCreationMaxVal                   float64

	prefillNonMediaSum, prefillNonMediaCount float64
	prefillNonMediaMinVal                    float64 = math.MaxFloat64
	prefillNonMediaMaxVal                    float64

	prefillMediaSum, prefillMediaCount float64
	prefillMediaMinVal                 float64 = math.MaxFloat64
	prefillMediaMaxVal                 float64

	ttftSum, ttftCount float64
	ttftMinVal         float64 = math.MaxFloat64
	ttftMaxVal         float64

	promptTokensSum, promptTokensCount float64
	promptTokensMinVal                 float64 = math.MaxFloat64
	promptTokensMaxVal                 float64

	reasoningTokensSum, reasoningTokensCount float64
	reasoningTokensMinVal                    float64 = math.MaxFloat64
	reasoningTokensMaxVal                    float64

	completionTokensSum, completionTokensCount float64
	completionTokensMinVal                     float64 = math.MaxFloat64
	completionTokensMaxVal                     float64

	outputTokensSum, outputTokensCount float64
	outputTokensMinVal                 float64 = math.MaxFloat64
	outputTokensMaxVal                 float64

	totalTokensSum, totalTokensCount float64
	totalTokensMinVal                float64 = math.MaxFloat64
	totalTokensMaxVal                float64

	tokensPerSecondSum, tokensPerSecondCount float64
	tokensPerSecondMinVal                    float64 = math.MaxFloat64
	tokensPerSecondMaxVal                    float64
)

type promMetrics struct {
	goroutines prometheus.Gauge
	requests   prometheus.Counter
	errors     prometheus.Counter
	panics     prometheus.Counter

	modelLoadAvg prometheus.Gauge
	modelLoadMin prometheus.Gauge
	modelLoadMax prometheus.Gauge

	modelLoadProjAvg prometheus.Gauge
	modelLoadProjMin prometheus.Gauge
	modelLoadProjMax prometheus.Gauge

	promptCreationAvg prometheus.Gauge
	promptCreationMin prometheus.Gauge
	promptCreationMax prometheus.Gauge

	prefillNonMediaAvg prometheus.Gauge
	prefillNonMediaMin prometheus.Gauge
	prefillNonMediaMax prometheus.Gauge

	prefillMediaAvg prometheus.Gauge
	prefillMediaMin prometheus.Gauge
	prefillMediaMax prometheus.Gauge

	ttftAvg prometheus.Gauge
	ttftMin prometheus.Gauge
	ttftMax prometheus.Gauge

	promptTokensAvg prometheus.Gauge
	promptTokensMin prometheus.Gauge
	promptTokensMax prometheus.Gauge

	reasoningTokensAvg prometheus.Gauge
	reasoningTokensMin prometheus.Gauge
	reasoningTokensMax prometheus.Gauge

	completionTokensAvg prometheus.Gauge
	completionTokensMin prometheus.Gauge
	completionTokensMax prometheus.Gauge

	outputTokensAvg prometheus.Gauge
	outputTokensMin prometheus.Gauge
	outputTokensMax prometheus.Gauge

	totalTokensAvg prometheus.Gauge
	totalTokensMin prometheus.Gauge
	totalTokensMax prometheus.Gauge

	tokensPerSecondAvg prometheus.Gauge
	tokensPerSecondMin prometheus.Gauge
	tokensPerSecondMax prometheus.Gauge
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

		modelLoadAvg: newGauge("model_load_avg", "Model load time average in seconds"),
		modelLoadMin: newGauge("model_load_min", "Model load time minimum in seconds"),
		modelLoadMax: newGauge("model_load_max", "Model load time maximum in seconds"),

		modelLoadProjAvg: newGauge("model_load_proj_avg", "Proj file load time average in seconds"),
		modelLoadProjMin: newGauge("model_load_proj_min", "Proj file load time minimum in seconds"),
		modelLoadProjMax: newGauge("model_load_proj_max", "Proj file load time maximum in seconds"),

		promptCreationAvg: newGauge("model_prompt_creation_avg", "Prompt creation time average in seconds"),
		promptCreationMin: newGauge("model_prompt_creation_min", "Prompt creation time minimum in seconds"),
		promptCreationMax: newGauge("model_prompt_creation_max", "Prompt creation time maximum in seconds"),

		prefillNonMediaAvg: newGauge("model_prefill_nonmedia_avg", "Prefill non-media time average in seconds"),
		prefillNonMediaMin: newGauge("model_prefill_nonmedia_min", "Prefill non-media time minimum in seconds"),
		prefillNonMediaMax: newGauge("model_prefill_nonmedia_max", "Prefill non-media time maximum in seconds"),

		prefillMediaAvg: newGauge("model_prefill_media_avg", "Prefill media time average in seconds"),
		prefillMediaMin: newGauge("model_prefill_media_min", "Prefill media time minimum in seconds"),
		prefillMediaMax: newGauge("model_prefill_media_max", "Prefill media time maximum in seconds"),

		ttftAvg: newGauge("model_ttft_avg", "Time to first token average in seconds"),
		ttftMin: newGauge("model_ttft_min", "Time to first token minimum in seconds"),
		ttftMax: newGauge("model_ttft_max", "Time to first token maximum in seconds"),

		promptTokensAvg: newGauge("usage_prompt_tokens_avg", "Prompt tokens average"),
		promptTokensMin: newGauge("usage_prompt_tokens_min", "Prompt tokens minimum"),
		promptTokensMax: newGauge("usage_prompt_tokens_max", "Prompt tokens maximum"),

		reasoningTokensAvg: newGauge("usage_reasoning_tokens_avg", "Reasoning tokens average"),
		reasoningTokensMin: newGauge("usage_reasoning_tokens_min", "Reasoning tokens minimum"),
		reasoningTokensMax: newGauge("usage_reasoning_tokens_max", "Reasoning tokens maximum"),

		completionTokensAvg: newGauge("usage_completion_tokens_avg", "Completion tokens average"),
		completionTokensMin: newGauge("usage_completion_tokens_min", "Completion tokens minimum"),
		completionTokensMax: newGauge("usage_completion_tokens_max", "Completion tokens maximum"),

		outputTokensAvg: newGauge("usage_output_tokens_avg", "Output tokens average"),
		outputTokensMin: newGauge("usage_output_tokens_min", "Output tokens minimum"),
		outputTokensMax: newGauge("usage_output_tokens_max", "Output tokens maximum"),

		totalTokensAvg: newGauge("usage_total_tokens_avg", "Total tokens average"),
		totalTokensMin: newGauge("usage_total_tokens_min", "Total tokens minimum"),
		totalTokensMax: newGauge("usage_total_tokens_max", "Total tokens maximum"),

		tokensPerSecondAvg: newGauge("usage_tokens_per_second_avg", "Tokens per second average"),
		tokensPerSecondMin: newGauge("usage_tokens_per_second_min", "Tokens per second minimum"),
		tokensPerSecondMax: newGauge("usage_tokens_per_second_max", "Tokens per second maximum"),
	}
}

func newGauge(name, help string) prometheus.Gauge {
	return promauto.NewGauge(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	})
}

// AddGoroutines refreshes the goroutine metric.
func AddGoroutines() int64 {
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
func AddModelFileLoadTime(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	modelLoadSum += secs
	modelLoadCount++
	m.modelLoadAvg.Set(modelLoadSum / modelLoadCount)

	if secs < modelLoadMinVal {
		modelLoadMinVal = secs
		m.modelLoadMin.Set(secs)
	}
	if secs > modelLoadMaxVal {
		modelLoadMaxVal = secs
		m.modelLoadMax.Set(secs)
	}
	mu.Unlock()
}

// AddProjFileLoadTime captures the specified duration for loading a proj file.
func AddProjFileLoadTime(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	modelLoadProjSum += secs
	modelLoadProjCount++
	m.modelLoadProjAvg.Set(modelLoadProjSum / modelLoadProjCount)

	if secs < modelLoadProjMinVal {
		modelLoadProjMinVal = secs
		m.modelLoadProjMin.Set(secs)
	}
	if secs > modelLoadProjMaxVal {
		modelLoadProjMaxVal = secs
		m.modelLoadProjMax.Set(secs)
	}
	mu.Unlock()
}

// AddPromptCreationTime captures the specified duration for creating a prompt.
func AddPromptCreationTime(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	promptCreationSum += secs
	promptCreationCount++
	m.promptCreationAvg.Set(promptCreationSum / promptCreationCount)

	if secs < promptCreationMinVal {
		promptCreationMinVal = secs
		m.promptCreationMin.Set(secs)
	}
	if secs > promptCreationMaxVal {
		promptCreationMaxVal = secs
		m.promptCreationMax.Set(secs)
	}
	mu.Unlock()
}

// AddPrefillNonMediaTime captures the specified duration for prefilling a non media call.
func AddPrefillNonMediaTime(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	prefillNonMediaSum += secs
	prefillNonMediaCount++
	m.prefillNonMediaAvg.Set(prefillNonMediaSum / prefillNonMediaCount)

	if secs < prefillNonMediaMinVal {
		prefillNonMediaMinVal = secs
		m.prefillNonMediaMin.Set(secs)
	}
	if secs > prefillNonMediaMaxVal {
		prefillNonMediaMaxVal = secs
		m.prefillNonMediaMax.Set(secs)
	}
	mu.Unlock()
}

// AddPrefillMediaTime captures the specified duration for prefilling a media call.
func AddPrefillMediaTime(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	prefillMediaSum += secs
	prefillMediaCount++
	m.prefillMediaAvg.Set(prefillMediaSum / prefillMediaCount)

	if secs < prefillMediaMinVal {
		prefillMediaMinVal = secs
		m.prefillMediaMin.Set(secs)
	}
	if secs > prefillMediaMaxVal {
		prefillMediaMaxVal = secs
		m.prefillMediaMax.Set(secs)
	}
	mu.Unlock()
}

// AddTimeToFirstToken captures the specified duration for ttft.
func AddTimeToFirstToken(duration time.Duration) {
	secs := duration.Seconds()

	mu.Lock()
	ttftSum += secs
	ttftCount++
	m.ttftAvg.Set(ttftSum / ttftCount)

	if secs < ttftMinVal {
		ttftMinVal = secs
		m.ttftMin.Set(secs)
	}
	if secs > ttftMaxVal {
		ttftMaxVal = secs
		m.ttftMax.Set(secs)
	}
	mu.Unlock()
}

// AddChatCompletionsUsage captures the specified usage values for chat-completions.
func AddChatCompletionsUsage(promptTokens, reasoningTokens, completionTokens, outputTokens, totalTokens int, tokensPerSecond float64) {
	mu.Lock()
	defer mu.Unlock()

	// Prompt tokens
	pt := float64(promptTokens)
	promptTokensSum += pt
	promptTokensCount++
	m.promptTokensAvg.Set(promptTokensSum / promptTokensCount)
	if pt < promptTokensMinVal {
		promptTokensMinVal = pt
		m.promptTokensMin.Set(pt)
	}
	if pt > promptTokensMaxVal {
		promptTokensMaxVal = pt
		m.promptTokensMax.Set(pt)
	}

	// Reasoning tokens
	rt := float64(reasoningTokens)
	reasoningTokensSum += rt
	reasoningTokensCount++
	m.reasoningTokensAvg.Set(reasoningTokensSum / reasoningTokensCount)
	if rt < reasoningTokensMinVal {
		reasoningTokensMinVal = rt
		m.reasoningTokensMin.Set(rt)
	}
	if rt > reasoningTokensMaxVal {
		reasoningTokensMaxVal = rt
		m.reasoningTokensMax.Set(rt)
	}

	// Completion tokens
	ct := float64(completionTokens)
	completionTokensSum += ct
	completionTokensCount++
	m.completionTokensAvg.Set(completionTokensSum / completionTokensCount)
	if ct < completionTokensMinVal {
		completionTokensMinVal = ct
		m.completionTokensMin.Set(ct)
	}
	if ct > completionTokensMaxVal {
		completionTokensMaxVal = ct
		m.completionTokensMax.Set(ct)
	}

	// Output tokens
	ot := float64(outputTokens)
	outputTokensSum += ot
	outputTokensCount++
	m.outputTokensAvg.Set(outputTokensSum / outputTokensCount)
	if ot < outputTokensMinVal {
		outputTokensMinVal = ot
		m.outputTokensMin.Set(ot)
	}
	if ot > outputTokensMaxVal {
		outputTokensMaxVal = ot
		m.outputTokensMax.Set(ot)
	}

	// Total tokens
	tt := float64(totalTokens)
	totalTokensSum += tt
	totalTokensCount++
	m.totalTokensAvg.Set(totalTokensSum / totalTokensCount)
	if tt < totalTokensMinVal {
		totalTokensMinVal = tt
		m.totalTokensMin.Set(tt)
	}
	if tt > totalTokensMaxVal {
		totalTokensMaxVal = tt
		m.totalTokensMax.Set(tt)
	}

	// Tokens per second
	tps := tokensPerSecond
	tokensPerSecondSum += tps
	tokensPerSecondCount++
	m.tokensPerSecondAvg.Set(tokensPerSecondSum / tokensPerSecondCount)
	if tps < tokensPerSecondMinVal {
		tokensPerSecondMinVal = tps
		m.tokensPerSecondMin.Set(tps)
	}
	if tps > tokensPerSecondMaxVal {
		tokensPerSecondMaxVal = tps
		m.tokensPerSecondMax.Set(tps)
	}
}
