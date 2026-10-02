package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

func runHybridState(rc *runContext) scenarioResult {
	model := rc.cfg.HybridModel
	result := scenarioResult{Models: []string{model}, Details: map[string]any{}}
	loaded, err := rc.ensureModel(model)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	result.Details["loaded_model"] = loaded
	if loaded.Slots != 1 {
		appendFailure(&result.Failures, "selected hybrid model must have exactly one slot; got %d", loaded.Slots)
	}
	engine, err := rc.engine(model)
	if err != nil {
		appendFailure(&result.Failures, "scheduler diagnostics: %v", err)
		return result
	}
	if engine == nil {
		result.Failures = append(result.Failures, "selected model has no scheduler diagnostics")
		return result
	}
	result.Details["scheduler"] = engine
	if len(engine.Slots) != 1 {
		appendFailure(&result.Failures, "scheduler reports %d slots, want 1", len(engine.Slots))
	}

	runID := fmt.Sprint(time.Now().UnixNano())
	baseMarker := "HYBRID-BASE-" + runID
	var prompt strings.Builder
	prompt.WriteString("Study this stable context, then obey the final instruction.\n")
	for number := range 48 {
		fmt.Fprintf(&prompt, "Context record %02d: amber cedar delta harbor quartz.\n", number)
	}
	fmt.Fprintf(&prompt, "Reply with exactly %s and nothing else.", baseMarker)
	baseMessages := []map[string]any{{"role": "user", "content": prompt.String()}}

	cold, coldErr := rc.completion(model, baseMessages, 64)
	repeat, repeatErr := rc.completion(model, baseMessages, 64)
	if coldErr != nil {
		appendFailure(&result.Failures, "cold deterministic request: %v", coldErr)
	} else if strings.TrimSpace(cold.Content) != baseMarker {
		appendFailure(&result.Failures, "cold response did not exactly match %s", baseMarker)
	}
	if repeatErr != nil {
		appendFailure(&result.Failures, "exact repeat: %v", repeatErr)
	} else {
		if repeat.Content != cold.Content {
			result.Failures = append(result.Failures, "exact deterministic repeat differed from cold response")
		}
		if repeat.Usage.PromptTokensDetails.CachedTokens <= 0 {
			result.Failures = append(result.Failures, "exact repeat reported no cached tokens")
		}
	}

	appendMarker := "HYBRID-APPEND-" + runID
	appendMessages := append([]map[string]any{}, baseMessages...)
	appendMessages = append(appendMessages,
		map[string]any{"role": "assistant", "content": cold.Content},
		map[string]any{"role": "user", "content": "Now reply with exactly " + appendMarker + " and nothing else."},
	)
	appended, appendErr := rc.completion(model, appendMessages, 64)
	if appendErr != nil {
		appendFailure(&result.Failures, "appended turn: %v", appendErr)
	} else {
		if strings.TrimSpace(appended.Content) != appendMarker {
			appendFailure(&result.Failures, "appended response did not exactly match %s", appendMarker)
		}
		if appended.Usage.PromptTokensDetails.CachedTokens <= 0 {
			result.Failures = append(result.Failures, "appended turn reported no cached tokens")
		}
	}

	staleMarker := "HYBRID-STALE-" + runID
	cancelBody := map[string]any{
		"model":           model,
		"stream":          true,
		"stream_options":  map[string]any{"include_usage": true},
		"temperature":     0,
		"seed":            rc.cfg.Seed,
		"max_tokens":      1024,
		"enable_thinking": false,
		"messages": []map[string]any{{
			"role":    "user",
			"content": fmt.Sprintf("Output %s on every line and continue until the output limit. Do not output anything else.", staleMarker),
		}},
	}
	cancelled, cancelErr := rc.client.stream(rc, rc.scenario, cancelBody, func(chunk chatChunk, _ time.Time) error {
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			return errStopStream
		}
		return nil
	})
	if !errors.Is(cancelErr, errStopStream) {
		appendFailure(&result.Failures, "cancelled stream: got %v, want client cancellation after content", cancelErr)
	}
	if cancelled.ContentCharacters == 0 {
		result.Failures = append(result.Failures, "cancelled stream produced no content before disconnect")
	}

	recoveryMarker := "HYBRID-RECOVERY-" + runID
	recovery, recoveryErr := rc.completion(model, []map[string]any{{
		"role": "user", "content": "Reply with exactly " + recoveryMarker + " and nothing else.",
	}}, 64)
	if recoveryErr != nil {
		appendFailure(&result.Failures, "post-cancellation recovery: %v", recoveryErr)
	} else {
		if strings.TrimSpace(recovery.Content) != recoveryMarker {
			appendFailure(&result.Failures, "recovery response did not exactly match %s", recoveryMarker)
		}
		if strings.Contains(recovery.Content, staleMarker) {
			result.Failures = append(result.Failures, "recovery response leaked the cancelled stream marker")
		}
	}

	completed := []completionResult{cold, repeat, appended, recovery}
	draftTokens, completionTokens := 0, 0
	weightedCoverage := 0.0
	requestSummaries := make([]map[string]any, 0, len(completed))
	labels := []string{"cold", "repeat", "appended", "recovery"}
	for index, completion := range completed {
		draftTokens += completion.Usage.DraftTokens
		completionTokens += completion.Usage.CompletionTokens
		weightedCoverage += completion.Usage.DraftCoverage * float64(completion.Usage.CompletionTokens)
		requestSummaries = append(requestSummaries, map[string]any{
			"name": labels[index], "trace_id": completion.TraceID, "response_id": completion.ResponseID,
			"characters": completion.Characters, "elapsed_seconds": completion.Elapsed,
			"finish_reason": completion.FinishReason, "usage": completion.Usage,
		})
	}
	coverage := 0.0
	if completionTokens > 0 {
		coverage = weightedCoverage / float64(completionTokens)
	}
	if engine.MTP && draftTokens <= 0 {
		result.Failures = append(result.Failures, "MTP scheduler produced no aggregate draft tokens")
	}
	if engine.MTP && coverage <= 0 {
		result.Failures = append(result.Failures, "MTP scheduler produced no aggregate draft coverage")
	}
	result.Details["requests"] = requestSummaries
	result.Details["cancelled_stream"] = map[string]any{
		"trace_id": cancelled.TraceID, "response_id": cancelled.ResponseID,
		"content_characters": cancelled.ContentCharacters, "elapsed_seconds": seconds(cancelled.FinishedAt.Sub(cancelled.StartedAt)),
	}
	result.Details["generation_usage"] = map[string]any{
		"mode":              map[bool]string{true: "mtp", false: "target-only"}[engine.MTP],
		"completion_tokens": completionTokens, "draft_tokens": draftTokens, "draft_coverage": coverage,
	}
	rc.artifacts.logf("hybrid-state: cold_cached=%d repeat_cached=%d appended_cached=%d cancelled_chars=%d recovery_chars=%d mode=%s",
		cold.Usage.PromptTokensDetails.CachedTokens, repeat.Usage.PromptTokensDetails.CachedTokens,
		appended.Usage.PromptTokensDetails.CachedTokens, cancelled.ContentCharacters, recovery.Characters,
		map[bool]string{true: "mtp", false: "target-only"}[engine.MTP])
	return result
}
