package main

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var longContextMarkerPattern = regexp.MustCompile(`LONGCTX-S\d+-(?:BEGIN|MIDDLE|END)-[0-9A-F]{8}`)

type longContextCompletion struct {
	TraceID         string  `json:"trace_id"`
	ResponseID      string  `json:"response_id,omitempty"`
	PromptTokens    int     `json:"prompt_tokens"`
	CachedTokens    int     `json:"cached_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	TTFTSeconds     float64 `json:"ttft_seconds"`
	TokensPerSecond float64 `json:"tokens_per_second"`
	ElapsedSeconds  float64 `json:"elapsed_seconds"`
	FinishReason    string  `json:"finish_reason,omitempty"`
	output          string
}

type longContextStage struct {
	TargetTokens     int                    `json:"target_tokens"`
	CalibratedTokens int                    `json:"calibrated_tokens,omitempty"`
	Markers          []string               `json:"markers,omitempty"`
	Status           string                 `json:"status"`
	Reason           string                 `json:"reason,omitempty"`
	Cold             *longContextCompletion `json:"cold,omitempty"`
	Warm             *longContextCompletion `json:"warm,omitempty"`
}

func runLongContext(rc *runContext) scenarioResult {
	model := rc.cfg.LongContextModel
	result := scenarioResult{Models: []string{model}, Details: map[string]any{}}
	loaded, err := rc.ensureModel(model)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	result.Details["loaded_model"] = loaded
	contextWindow, contextSource := rc.configuredContext(model)
	result.Details["configured_context"] = contextWindow
	result.Details["context_source"] = contextSource

	known := make(map[string]bool)
	for _, target := range rc.cfg.LongContextStages {
		for _, marker := range longContextMarkers(target, rc.cfg.Seed) {
			known[marker] = true
		}
	}

	stages := make([]longContextStage, 0, len(rc.cfg.LongContextStages))
	ran := 0
	for _, target := range rc.cfg.LongContextStages {
		stage := longContextStage{TargetTokens: target}
		if contextWindow > 0 && target > contextWindow {
			stage.Status = "SKIP"
			stage.Reason = fmt.Sprintf("target exceeds configured context %d", contextWindow)
			stages = append(stages, stage)
			rc.artifacts.logf("long-context stage %d: SKIP (%s)", target, stage.Reason)
			continue
		}

		prompt, calibrated, markers, err := makeLongContextPrompt(rc, model, target)
		if err != nil {
			stage.Status, stage.Reason = "FAIL", err.Error()
			stages = append(stages, stage)
			appendFailure(&result.Failures, "stage %d calibration: %v", target, err)
			break
		}
		stage.CalibratedTokens, stage.Markers = calibrated, markers
		cold, err := streamLongContext(rc, model, prompt)
		if err == nil {
			err = verifyLongContextOutput(cold.output, markers, known)
		}
		if err != nil {
			stage.Status, stage.Reason = "FAIL", err.Error()
			stages = append(stages, stage)
			appendFailure(&result.Failures, "stage %d cold retrieval: %v", target, err)
			break
		}
		warm, err := streamLongContext(rc, model, prompt)
		if err == nil {
			err = verifyLongContextOutput(warm.output, markers, known)
		}
		if err == nil && warm.CachedTokens <= 0 {
			err = fmt.Errorf("warm repeat reported cached_tokens=0")
		}
		if err == nil && warm.output != cold.output {
			err = fmt.Errorf("warm deterministic output differs from cold output")
		}
		stage.Cold, stage.Warm = &cold, &warm
		if err != nil {
			stage.Status, stage.Reason = "FAIL", err.Error()
			stages = append(stages, stage)
			appendFailure(&result.Failures, "stage %d warm retrieval: %v", target, err)
			break
		}
		stage.Status = "PASS"
		stages = append(stages, stage)
		ran++
		rc.artifacts.logf("long-context stage %d: PASS prompt=%d cached=%d ttft=%.3fs tps=%.1f",
			target, warm.PromptTokens, warm.CachedTokens, warm.TTFTSeconds, warm.TokensPerSecond)
	}
	if ran == 0 && len(result.Failures) == 0 {
		result.Failures = append(result.Failures, "no requested long-context stage ran")
	}
	result.Details["stages"] = stages
	return result
}

func (rc *runContext) configuredContext(model string) (int, string) {
	var detail struct {
		ModelConfig map[string]any `json:"model_config"`
	}
	path := "/v1/kronk/models/" + url.PathEscape(model)
	if _, err := rc.client.json(rc, rc.scenario, http.MethodGet, path, nil, &detail); err != nil {
		return 0, "unavailable: " + err.Error()
	}
	for _, key := range []string{"context-window", "context_window"} {
		if value, ok := detail.ModelConfig[key].(float64); ok && value > 0 {
			return int(value), "model management detail"
		}
	}
	return 0, "unavailable in model management detail"
}

func makeLongContextPrompt(rc *runContext, model string, target int) (string, int, []string, error) {
	markers := longContextMarkers(target, rc.cfg.Seed)
	contentTarget := max(1, target-rc.cfg.LongContextMaxTokens-64)
	intro := "Long-context retrieval test. Memorize the three exact marker strings embedded below. Ignore the filler.\n"
	ending := "\nReturn only all three marker strings, each exactly once, in their original order. Do not explain.\n"
	filler := " archival ledger quartz orbit telemetry cedar delta nominal."
	candidate := func(repetitions int) string {
		third := repetitions / 3
		return intro + markers[0] + strings.Repeat(filler, third) + markers[1] +
			strings.Repeat(filler, repetitions-2*third) + markers[2] + strings.Repeat(filler, third) + ending
	}

	low, high := 0, max(1, contentTarget/3)
	for {
		count, err := rc.tokenCount(model, candidate(high), false)
		if err != nil {
			return "", 0, nil, err
		}
		if count >= contentTarget {
			break
		}
		high *= 2
	}
	for low < high {
		middle := (low + high + 1) / 2
		count, err := rc.tokenCount(model, candidate(middle), false)
		if err != nil {
			return "", 0, nil, err
		}
		if count <= contentTarget {
			low = middle
		} else {
			high = middle - 1
		}
	}
	prompt := candidate(low)
	count, err := rc.tokenCount(model, prompt, false)
	return prompt, count, markers, err
}

func longContextMarkers(stage int, seed int64) []string {
	places := []string{"BEGIN", "MIDDLE", "END"}
	markers := make([]string, len(places))
	for index, place := range places {
		value := uint32(uint64(stage)*2654435761 + uint64(seed) + uint64(index))
		markers[index] = fmt.Sprintf("LONGCTX-S%d-%s-%08X", stage, place, value)
	}
	return markers
}

func streamLongContext(rc *runContext, model, prompt string) (longContextCompletion, error) {
	body := map[string]any{
		"model": model, "stream": true, "stream_options": map[string]any{"include_usage": true},
		"temperature": 0, "seed": rc.cfg.Seed, "max_tokens": rc.cfg.LongContextMaxTokens,
		"enable_thinking": false, "messages": []map[string]any{{"role": "user", "content": prompt}},
	}
	stream, err := rc.client.stream(rc, rc.scenario, body, nil)
	result := longContextCompletion{
		TraceID: stream.TraceID, ResponseID: stream.ResponseID, PromptTokens: stream.Usage.PromptTokens,
		CachedTokens: stream.Usage.PromptTokensDetails.CachedTokens, OutputTokens: stream.Usage.CompletionTokens,
		ElapsedSeconds: seconds(stream.FinishedAt.Sub(stream.StartedAt)), FinishReason: stream.FinishReason, output: stream.Content,
	}
	if err != nil {
		return result, err
	}
	if stream.ContentCharacters == 0 || stream.FirstContentAt.IsZero() {
		return result, fmt.Errorf("completion returned no text")
	}
	result.TTFTSeconds = seconds(stream.FirstContentAt.Sub(stream.StartedAt))
	decodeDuration := stream.FinishedAt.Sub(stream.FirstContentAt).Seconds()
	if decodeDuration > 0 {
		result.TokensPerSecond = float64(stream.Usage.CompletionTokens) / decodeDuration
	}
	return result, nil
}

func verifyLongContextOutput(output string, expected []string, known map[string]bool) error {
	found := longContextMarkerPattern.FindAllString(output, -1)
	if strings.Join(found, "\x00") != strings.Join(expected, "\x00") {
		return fmt.Errorf("marker validation failed: expected=%v found=%v", expected, found)
	}
	for marker := range known {
		if !contains(expected, marker) && strings.Contains(output, marker) {
			return fmt.Errorf("response contained foreign marker %s", marker)
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	return slices.Contains(values, target)
}
