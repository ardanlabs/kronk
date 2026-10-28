package main

import (
	"math"
	"slices"
)

// Freeze a representative volume per cell, not per invocation.
// Values may need to be updated after some important change in the tested sequence.
const (
	// MoE/1s/IMC+: both MTP modes share the same reference pair.
	moe1SlotIMCTextInputTokens        = 10708
	moe1SlotIMCTextGenerationTokens   = 1332
	moe1SlotIMCImagesInputTokens      = 11674
	moe1SlotIMCImagesGenerationTokens = 1001

	// MoE/1s/IMC-: input is shared; generation is calibrated per MTP mode.
	moe1SlotNoIMCTextInputTokens              = 51405
	moe1SlotNoIMCTextMTPOnGenerationTokens    = 1085
	moe1SlotNoIMCTextMTPOffGenerationTokens   = 1117
	moe1SlotNoIMCImagesInputTokens            = 53095
	moe1SlotNoIMCImagesMTPOnGenerationTokens  = 958
	moe1SlotNoIMCImagesMTPOffGenerationTokens = 865

	// MoE/2s: initial effective input/generation volumes, one sample per cell.
	moe2SlotIMCTextMTPOnInputTokens           = 11396
	moe2SlotIMCTextMTPOnGenerationTokens      = 2047
	moe2SlotIMCImagesMTPOnInputTokens         = 12379
	moe2SlotIMCImagesMTPOnGenerationTokens    = 1864
	moe2SlotIMCTextMTPOffInputTokens          = 11223
	moe2SlotIMCTextMTPOffGenerationTokens     = 1562
	moe2SlotIMCImagesMTPOffInputTokens        = 12254
	moe2SlotIMCImagesMTPOffGenerationTokens   = 1515
	moe2SlotNoIMCTextMTPOnInputTokens         = 72619
	moe2SlotNoIMCTextMTPOnGenerationTokens    = 1556
	moe2SlotNoIMCImagesMTPOnInputTokens       = 74370
	moe2SlotNoIMCImagesMTPOnGenerationTokens  = 1576
	moe2SlotNoIMCTextMTPOffInputTokens        = 72644
	moe2SlotNoIMCTextMTPOffGenerationTokens   = 1595
	moe2SlotNoIMCImagesMTPOffInputTokens      = 74388
	moe2SlotNoIMCImagesMTPOffGenerationTokens = 1613

	// Dense Q8: provisional operations-v3 calibration from output/new-f87f7d4,
	// pending a fresh familiar-word/names workload run. Each cell uses rounded
	// medians of completed-request effective input and completion-minus-one;
	// cancelled requests and warmup are excluded (same accounting as referenceTime).
	dense1SlotIMCTextMTPOnInputTokens           = 10584
	dense1SlotIMCTextMTPOnGenerationTokens      = 492
	dense1SlotIMCImagesMTPOnInputTokens         = 11627
	dense1SlotIMCImagesMTPOnGenerationTokens    = 560
	dense1SlotIMCTextMTPOffInputTokens          = 10572
	dense1SlotIMCTextMTPOffGenerationTokens     = 449
	dense1SlotIMCImagesMTPOffInputTokens        = 11644
	dense1SlotIMCImagesMTPOffGenerationTokens   = 645
	dense1SlotNoIMCTextMTPOnInputTokens         = 51371
	dense1SlotNoIMCTextMTPOnGenerationTokens    = 467
	dense1SlotNoIMCImagesMTPOnInputTokens       = 53139
	dense1SlotNoIMCImagesMTPOnGenerationTokens  = 592
	dense1SlotNoIMCTextMTPOffInputTokens        = 51360
	dense1SlotNoIMCTextMTPOffGenerationTokens   = 427
	dense1SlotNoIMCImagesMTPOffInputTokens      = 53139
	dense1SlotNoIMCImagesMTPOffGenerationTokens = 599
	dense2SlotIMCTextMTPOnInputTokens           = 10904
	dense2SlotIMCTextMTPOnGenerationTokens      = 768
	dense2SlotIMCImagesMTPOnInputTokens         = 12000
	dense2SlotIMCImagesMTPOnGenerationTokens    = 869
	dense2SlotIMCTextMTPOffInputTokens          = 10887
	dense2SlotIMCTextMTPOffGenerationTokens     = 745
	dense2SlotIMCImagesMTPOffInputTokens        = 11971
	dense2SlotIMCImagesMTPOffGenerationTokens   = 834
	dense2SlotNoIMCTextMTPOnInputTokens         = 72340
	dense2SlotNoIMCTextMTPOnGenerationTokens    = 830
	dense2SlotNoIMCImagesMTPOnInputTokens       = 74103
	dense2SlotNoIMCImagesMTPOnGenerationTokens  = 860
	dense2SlotNoIMCTextMTPOffInputTokens        = 72319
	dense2SlotNoIMCTextMTPOffGenerationTokens   = 736
	// One complete measured repetition: the sole finding was a stale Northport
	// mention from this same conversation, not missing/failed execution evidence.
	// The cell remains FAIL; its unrun second repetition is not a sample.
	dense2SlotNoIMCImagesMTPOffInputTokens      = 74073
	dense2SlotNoIMCImagesMTPOffGenerationTokens = 821
)

type normalizationReference struct {
	InputTokens      int `json:"input_tokens"`
	GenerationTokens int `json:"generation_tokens"`
}

func denseReferenceTokens(images, noCache, targetOnly, twoSlots bool) normalizationReference {
	if twoSlots {
		switch {
		case noCache && images && targetOnly:
			return normalizationReference{dense2SlotNoIMCImagesMTPOffInputTokens, dense2SlotNoIMCImagesMTPOffGenerationTokens}
		case noCache && images:
			return normalizationReference{dense2SlotNoIMCImagesMTPOnInputTokens, dense2SlotNoIMCImagesMTPOnGenerationTokens}
		case noCache && targetOnly:
			return normalizationReference{dense2SlotNoIMCTextMTPOffInputTokens, dense2SlotNoIMCTextMTPOffGenerationTokens}
		case noCache:
			return normalizationReference{dense2SlotNoIMCTextMTPOnInputTokens, dense2SlotNoIMCTextMTPOnGenerationTokens}
		case images && targetOnly:
			return normalizationReference{dense2SlotIMCImagesMTPOffInputTokens, dense2SlotIMCImagesMTPOffGenerationTokens}
		case images:
			return normalizationReference{dense2SlotIMCImagesMTPOnInputTokens, dense2SlotIMCImagesMTPOnGenerationTokens}
		case targetOnly:
			return normalizationReference{dense2SlotIMCTextMTPOffInputTokens, dense2SlotIMCTextMTPOffGenerationTokens}
		default:
			return normalizationReference{dense2SlotIMCTextMTPOnInputTokens, dense2SlotIMCTextMTPOnGenerationTokens}
		}
	}
	switch {
	case noCache && images && targetOnly:
		return normalizationReference{dense1SlotNoIMCImagesMTPOffInputTokens, dense1SlotNoIMCImagesMTPOffGenerationTokens}
	case noCache && images:
		return normalizationReference{dense1SlotNoIMCImagesMTPOnInputTokens, dense1SlotNoIMCImagesMTPOnGenerationTokens}
	case noCache && targetOnly:
		return normalizationReference{dense1SlotNoIMCTextMTPOffInputTokens, dense1SlotNoIMCTextMTPOffGenerationTokens}
	case noCache:
		return normalizationReference{dense1SlotNoIMCTextMTPOnInputTokens, dense1SlotNoIMCTextMTPOnGenerationTokens}
	case images && targetOnly:
		return normalizationReference{dense1SlotIMCImagesMTPOffInputTokens, dense1SlotIMCImagesMTPOffGenerationTokens}
	case images:
		return normalizationReference{dense1SlotIMCImagesMTPOnInputTokens, dense1SlotIMCImagesMTPOnGenerationTokens}
	case targetOnly:
		return normalizationReference{dense1SlotIMCTextMTPOffInputTokens, dense1SlotIMCTextMTPOffGenerationTokens}
	default:
		return normalizationReference{dense1SlotIMCTextMTPOnInputTokens, dense1SlotIMCTextMTPOnGenerationTokens}
	}
}

func referenceTokens(images, noCache, targetOnly, twoSlots bool) normalizationReference {
	if twoSlots {
		switch {
		case noCache && images && targetOnly:
			return normalizationReference{moe2SlotNoIMCImagesMTPOffInputTokens, moe2SlotNoIMCImagesMTPOffGenerationTokens}
		case noCache && images:
			return normalizationReference{moe2SlotNoIMCImagesMTPOnInputTokens, moe2SlotNoIMCImagesMTPOnGenerationTokens}
		case noCache && targetOnly:
			return normalizationReference{moe2SlotNoIMCTextMTPOffInputTokens, moe2SlotNoIMCTextMTPOffGenerationTokens}
		case noCache:
			return normalizationReference{moe2SlotNoIMCTextMTPOnInputTokens, moe2SlotNoIMCTextMTPOnGenerationTokens}
		case images && targetOnly:
			return normalizationReference{moe2SlotIMCImagesMTPOffInputTokens, moe2SlotIMCImagesMTPOffGenerationTokens}
		case images:
			return normalizationReference{moe2SlotIMCImagesMTPOnInputTokens, moe2SlotIMCImagesMTPOnGenerationTokens}
		case targetOnly:
			return normalizationReference{moe2SlotIMCTextMTPOffInputTokens, moe2SlotIMCTextMTPOffGenerationTokens}
		default:
			return normalizationReference{moe2SlotIMCTextMTPOnInputTokens, moe2SlotIMCTextMTPOnGenerationTokens}
		}
	}
	if images {
		if noCache {
			generated := moe1SlotNoIMCImagesMTPOnGenerationTokens
			if targetOnly {
				generated = moe1SlotNoIMCImagesMTPOffGenerationTokens
			}
			return normalizationReference{InputTokens: moe1SlotNoIMCImagesInputTokens, GenerationTokens: generated}
		}
		return normalizationReference{InputTokens: moe1SlotIMCImagesInputTokens, GenerationTokens: moe1SlotIMCImagesGenerationTokens}
	}
	if noCache {
		generated := moe1SlotNoIMCTextMTPOnGenerationTokens
		if targetOnly {
			generated = moe1SlotNoIMCTextMTPOffGenerationTokens
		}
		return normalizationReference{InputTokens: moe1SlotNoIMCTextInputTokens, GenerationTokens: generated}
	}
	return normalizationReference{InputTokens: moe1SlotIMCTextInputTokens, GenerationTokens: moe1SlotIMCTextGenerationTokens}
}

// Reference time normalizes completed-request processing, not whole-workload
// wall time. Prefill includes cache work through the first sampled token.
// Two-slot processing durations overlap: their sum is not elapsed wall time.
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
	Repetition           int                 `json:"repetition"`
	Status               string              `json:"status"`
	Findings             []string            `json:"findings"`
	FindingsByCategory   map[string][]string `json:"findings_by_category"`
	SystemTokens         int                 `json:"system_tokens"`
	SystemSHA256         string              `json:"system_sha256"`
	WorkloadSeconds      float64             `json:"workload_seconds"`
	CancelReleaseSeconds float64             `json:"cancel_release_seconds"`
	Results              []map[string]any    `json:"results"`
}

func (r report) repetitionSummary(index int) repetitionSummary {
	summary := repetitionSummary{
		Repetition: index, Status: r.Status, Findings: append([]string{}, r.Findings...),
		FindingsByCategory: classifyFindings(r.Findings),
		SystemTokens:       r.SystemTokens, SystemSHA256: r.SystemSHA256, WorkloadSeconds: r.WorkloadSeconds,
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
