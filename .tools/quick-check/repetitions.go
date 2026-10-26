package main

import (
	"math"
	"slices"
)

type normalizationReference struct {
	InputTokens      int `json:"input_tokens"`
	GenerationTokens int `json:"generation_tokens"`
}

// Freeze a representative volume per cell, not per invocation. Text references
// come from the initial nine observations; image references are rounded medians
// from new-0466efe's two repetitions with media System-cache reuse. Generation
// excludes the first token per response, matching Kronk's TPS clock.
func referenceTokens(images bool) normalizationReference {
	if images {
		return normalizationReference{InputTokens: 11674, GenerationTokens: 1001}
	}
	return normalizationReference{InputTokens: 10708, GenerationTokens: 1332}
}

// Reference time normalizes completed-request processing, not whole-workload
// wall time. Prefill includes cache work through the first sampled token.
func (r report) referenceTime() (float64, bool) {
	if r.ReferenceTokens.InputTokens <= 0 || r.ReferenceTokens.GenerationTokens <= 0 {
		return 0, false
	}
	var input, generated int
	var prefill, generation float64
	for _, res := range r.Results {
		if res.Cancelled {
			continue
		}
		p, g := res.ServerTimings["slot_prefill_ttft_seconds"], res.ServerTimings["generation_seconds"]
		if p <= 0 || g <= 0 || res.Usage.CompletionTokens < 1 {
			return 0, false
		}
		reused := res.Execution.ReusedTokens
		if res.MediaPlan && res.CacheReason != "system-cache-prefix" {
			// Media anchors have distinct logical/physical extents. A restored
			// text-only System prefix instead uses its actual reused token count.
			reused = res.MediaAnchorKV
		}
		input += res.Usage.PromptTokens - reused
		generated += res.Usage.CompletionTokens - 1
		prefill += p
		generation += g
	}
	if input <= 0 || generated <= 0 {
		return 0, false
	}
	return float64(r.ReferenceTokens.InputTokens)*prefill/float64(input) + float64(r.ReferenceTokens.GenerationTokens)*generation/float64(generated), true
}

type repetitionSummary struct {
	Repetition           int              `json:"repetition"`
	Status               string           `json:"status"`
	Findings             []string         `json:"findings"`
	SystemTokens         int              `json:"system_tokens"`
	SystemSHA256         string           `json:"system_sha256"`
	WorkloadSeconds      float64          `json:"workload_seconds"`
	CancelReleaseSeconds float64          `json:"cancel_release_seconds"`
	Results              []map[string]any `json:"results"`
}

func (r report) repetitionSummary(index int) repetitionSummary {
	summary := repetitionSummary{
		Repetition: index, Status: r.Status, Findings: append([]string{}, r.Findings...),
		SystemTokens: r.SystemTokens, SystemSHA256: r.SystemSHA256, WorkloadSeconds: r.WorkloadSeconds,
		CancelReleaseSeconds: r.CancelReleaseSeconds, Results: []map[string]any{},
	}
	for _, res := range r.Results {
		summary.Results = append(summary.Results, r.phaseSummary(res))
	}
	return summary
}

type measurement struct {
	Samples      int      `json:"samples"`
	Median       float64  `json:"median"`
	Min          float64  `json:"min"`
	Max          float64  `json:"max"`
	SampleStdDev *float64 `json:"sample_stddev"`
}

// Summarize comparable timings, never pool first-use requests with the warmed
// workload or hide a failed/incomplete repetition in a passing aggregate.
// Individual token counts and server stage durations remain in each repetition.
func (r report) measurements() map[string]map[string]measurement {
	if len(r.Findings) > 0 || len(r.Repetitions) == 0 || len(r.Repetitions) != r.RepetitionsRequested {
		return nil
	}
	samples := make(map[string]map[string][]float64)
	add := func(phase, metric string, value float64) {
		if samples[phase] == nil {
			samples[phase] = make(map[string][]float64)
		}
		samples[phase][metric] = append(samples[phase][metric], value)
	}
	for _, repetition := range r.Repetitions {
		if repetition.Status != "PASS" {
			return nil
		}
		add("workload", "elapsed_seconds", repetition.WorkloadSeconds)
		add("workload", "cancel_release_seconds", repetition.CancelReleaseSeconds)
		if seconds, ok := repetition.referenceTime(); ok {
			add("workload", "reference_seconds", seconds)
		}
		for _, res := range repetition.Results {
			add(res.Phase, "elapsed_seconds", res.Elapsed)
			add(res.Phase, "first_content_seconds", res.FirstContent)
			if !res.Cancelled && res.Usage.CompletionTokens > 0 {
				add(res.Phase, "server_output_tokens_per_second", res.Usage.ServerTPS)
			}
		}
	}
	if len(samples["workload"]["reference_seconds"]) != len(r.Repetitions) {
		delete(samples["workload"], "reference_seconds")
	}
	measurements := make(map[string]map[string]measurement)
	for phase, metrics := range samples {
		measurements[phase] = make(map[string]measurement)
		for metric, values := range metrics {
			measurements[phase][metric] = summarize(values)
		}
	}
	return measurements
}

func summarize(values []float64) measurement {
	slices.Sort(values)
	n := len(values)
	median := values[n/2]
	if n%2 == 0 {
		median = (values[n/2-1] + median) / 2
	}
	m := measurement{Samples: n, Median: median, Min: values[0], Max: values[n-1]}
	if n > 1 {
		var mean, squaredDeviations float64
		for _, value := range values {
			mean += value
		}
		mean /= float64(n)
		for _, value := range values {
			squaredDeviations += (value - mean) * (value - mean)
		}
		m.SampleStdDev = new(math.Sqrt(squaredDeviations / float64(n-1)))
	}
	return m
}
