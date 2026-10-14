package main

import "math"

type performanceGroup struct {
	Scenario string `json:"scenario"`
	Cell     string `json:"cell"`
	Workload string `json:"workload,omitempty"`
	Status   string `json:"scenario_status"`
	Scope    string `json:"scope"`
	Turn     int    `json:"turn,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Session  int    `json:"session,omitempty"`
}

type performanceAggregate struct {
	performanceGroup
	Metrics map[string]*repetitionStatistics `json:"metrics"`
}

type repetitionStatistics struct {
	Count                   int      `json:"count"`
	MissingRepetitions      int      `json:"missing_repetitions"`
	Repetitions             []int    `json:"repetitions"`
	Mean                    float64  `json:"mean"`
	SampleStandardDeviation *float64 `json:"sample_standard_deviation,omitempty"`
	m2                      float64
}

// Match repetitions, not requests within one repetition. Failed runs stay separate.
func aggregatePerformance(summary *runSummary) {
	if summary.Configuration.Repetitions <= 1 {
		return
	}
	groups := make(map[performanceGroup]int)
	summary.PerformanceAggregates = nil
	add := func(group performanceGroup, repetition int, metric string, value float64) {
		index, exists := groups[group]
		if !exists {
			index = len(summary.PerformanceAggregates)
			groups[group] = index
			summary.PerformanceAggregates = append(summary.PerformanceAggregates, performanceAggregate{performanceGroup: group, Metrics: make(map[string]*repetitionStatistics)})
		}
		metrics := summary.PerformanceAggregates[index].Metrics
		stats := metrics[metric]
		if stats == nil {
			stats = &repetitionStatistics{}
			metrics[metric] = stats
		}
		stats.Count++
		stats.MissingRepetitions = summary.Configuration.Repetitions - stats.Count
		stats.Repetitions = append(stats.Repetitions, repetition)
		delta := value - stats.Mean
		stats.Mean += delta / float64(stats.Count)
		stats.m2 += delta * (value - stats.Mean)
		if stats.Count > 1 {
			deviation := math.Sqrt(stats.m2 / float64(stats.Count-1))
			stats.SampleStandardDeviation = &deviation
		}
	}
	streamMetrics := func(group performanceGroup, repetition int, timing streamPerformance, tokens int, serverTPS, serverTTFT *float64) {
		add(group, repetition, "elapsed_seconds", timing.ElapsedSeconds)
		add(group, repetition, "completion_tokens", float64(tokens))
		if !timing.FirstContentAt.IsZero() {
			add(group, repetition, "first_content_seconds", timing.FirstContentSeconds)
			add(group, repetition, "generation_seconds", timing.GenerationSeconds)
			if timing.GenerationSeconds > 0 {
				add(group, repetition, "client_output_tokens_per_second", timing.OutputTokensPerSecond)
			}
		}
		if serverTPS != nil {
			add(group, repetition, "server_tokens_per_second", *serverTPS)
		}
		if serverTTFT != nil {
			add(group, repetition, "server_ttft_ms", *serverTTFT)
		}
	}
	for _, result := range summary.Scenarios {
		workload, _ := result.Details["workload"].(string)
		group := performanceGroup{Scenario: result.Name, Cell: result.Cell, Workload: workload, Status: result.Status, Scope: "scenario"}
		if result.Status == "PASS" {
			add(group, result.Repetition, "duration_seconds", result.DurationSeconds)
		}
		if phases, ok := result.Details["turns"].([]batchTurnSummary); ok {
			for _, phase := range phases {
				if !phase.Complete {
					continue
				}
				group.Scope, group.Turn, group.Phase = "batch-phase", phase.Turn, phase.Phase
				add(group, result.Repetition, "elapsed_seconds", phase.ElapsedSeconds)
				add(group, result.Repetition, "output_tokens", float64(phase.OutputTokens))
				if phase.ElapsedSeconds > 0 {
					add(group, result.Repetition, "output_tokens_per_second", phase.OutputTokensPerSecond)
				}
			}
		}
		if requests, ok := result.Details["requests"].([]batchRequestResult); ok {
			for _, request := range requests {
				if request.Error != "" || request.FinishReason == "" {
					continue
				}
				group.Scope, group.Turn, group.Phase, group.Session = "batch-request", request.Turn, request.Phase, request.Session
				streamMetrics(group, result.Repetition, request.streamPerformance, request.CompletionTokens, request.ServerTokensPerSecond, request.ServerTTFTMS)
			}
		}
		if prefill, ok := result.Details["concurrent_prefill"].(mediaPrefillResult); ok {
			group.Turn, group.Phase, group.Session = 0, "", 0
			for _, stream := range []struct {
				scope string
				mediaStreamResult
			}{{"media-generation", prefill.Generation}, {"media-image", prefill.Image}} {
				if stream.Error != "" || stream.FinishReason == "" {
					continue
				}
				group.Scope = stream.scope
				streamMetrics(group, result.Repetition, stream.Performance, stream.Usage.CompletionTokens, stream.Usage.TokensPerSecond, stream.Usage.TimeToFirstTokenMS)
			}
			if prefill.WorkloadComplete && prefill.GenerationOverlap > 0 && prefill.PrefillMaximumGapSeconds > 0 {
				group.Scope = "media-prefill"
				add(group, result.Repetition, "baseline_maximum_gap_seconds", prefill.BaselineMaximumGapSeconds)
				add(group, result.Repetition, "prefill_maximum_gap_seconds", prefill.PrefillMaximumGapSeconds)
			}
		}
	}
}
