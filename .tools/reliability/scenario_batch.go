package main

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

const batchCanaryRepetitions = 8

var batchMarkerPattern = regexp.MustCompile(`BATCH-SESSION-\d+-TURN-\d+`)

type batchRequestResult struct {
	Session             int     `json:"session"`
	Turn                int     `json:"turn"`
	TraceID             string  `json:"trace_id,omitempty"`
	ResponseID          string  `json:"response_id,omitempty"`
	ElapsedSeconds      float64 `json:"elapsed_seconds"`
	FirstContentSeconds float64 `json:"first_content_seconds"`
	PromptTokens        int     `json:"prompt_tokens"`
	CachedTokens        int     `json:"cached_tokens"`
	CompletionTokens    int     `json:"completion_tokens"`
	DraftTokens         int     `json:"draft_tokens,omitempty"`
	DraftCoverage       float64 `json:"draft_coverage,omitempty"`
	DraftDisableReason  string  `json:"draft_disable_reason,omitempty"`
	FinishReason        string  `json:"finish_reason,omitempty"`
	Error               string  `json:"error,omitempty"`
	firstContentAt      time.Time
	finishedAt          time.Time
}

type batchTurnSummary struct {
	Turn               int     `json:"turn"`
	Completed          int     `json:"completed"`
	MaximumConcurrency int     `json:"maximum_concurrency"`
	QueuedRequests     int     `json:"queued_requests"`
	MinimumPrompt      int     `json:"minimum_prompt_tokens"`
	MinimumCached      int     `json:"minimum_cached_tokens"`
	SlowestSeconds     float64 `json:"slowest_seconds"`
}

func runBatch(rc *runContext) scenarioResult {
	model := rc.cfg.BatchModel
	result := scenarioResult{Models: []string{model}, Details: map[string]any{}}
	loaded, err := rc.ensureModel(model)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	result.Details["loaded_model"] = loaded
	if loaded.Slots != rc.cfg.BatchSlots {
		appendFailure(&result.Failures, "loaded model reports %d slots, want %d", loaded.Slots, rc.cfg.BatchSlots)
		return result
	}

	records, err := calibrateBatchRecords(rc, model)
	if err != nil {
		appendFailure(&result.Failures, "calibrate batch turn: %v", err)
		return result
	}
	calibrated, err := rc.tokenCount(model, batchRecordPrompt(0, 1, records), false)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	lastCanary := fmt.Sprintf("%s-TURN-%d", batchConversationKey(rc.cfg.BatchConversations-1), rc.cfg.BatchTurns)
	canaryTokens, err := rc.tokenCount(model, strings.TrimSpace(strings.Repeat(lastCanary+" ", batchCanaryRepetitions)), false)
	if err != nil {
		result.Failures = append(result.Failures, err.Error())
		return result
	}
	if canaryTokens > rc.cfg.BatchMaxTokens {
		appendFailure(&result.Failures, "%d canaries require %d tokens, exceeding max %d", batchCanaryRepetitions, canaryTokens, rc.cfg.BatchMaxTokens)
		return result
	}
	rc.artifacts.logf("batch model=%s slots=%d conversations=%d calibrated_turn=%d canary_tokens=%d/%d",
		model, rc.cfg.BatchSlots, rc.cfg.BatchConversations, calibrated, canaryTokens, rc.cfg.BatchMaxTokens)

	conversations := make([][]map[string]any, rc.cfg.BatchConversations)
	for session := range rc.cfg.BatchConversations {
		conversations[session] = []map[string]any{{
			"role":    "system",
			"content": fmt.Sprintf("You are participating in a deterministic long-running load test. Keep this conversation separate: %s.", batchConversationKey(session)),
		}}
	}

	var allRequests []batchRequestResult
	var turns []batchTurnSummary
	for turn := 1; turn <= rc.cfg.BatchTurns; turn++ {
		turnRecords := records
		if turn == rc.cfg.BatchTurns {
			minimumCurrent := 0
			for _, messages := range conversations {
				count, err := rc.tokenCount(model, batchMessageText(messages), false)
				if err != nil {
					appendFailure(&result.Failures, "measure final history: %v", err)
					break
				}
				if minimumCurrent == 0 || count < minimumCurrent {
					minimumCurrent = count
				}
			}
			remaining := rc.cfg.BatchTargetTokens - minimumCurrent
			if remaining > rc.cfg.BatchTokensPerTurn {
				turnRecords = max(records, records*remaining/rc.cfg.BatchTokensPerTurn)
			}
		}
		if len(result.Failures) > 0 {
			break
		}

		requests, updated := runBatchTurn(rc, model, turn, turnRecords, conversations)
		allRequests = append(allRequests, requests...)
		turnSummary := summarizeBatchTurn(turn, requests)
		turns = append(turns, turnSummary)
		for _, request := range requests {
			if request.Error != "" {
				appendFailure(&result.Failures, "session %d turn %d: %s", request.Session, turn, request.Error)
			}
		}
		if turnSummary.MaximumConcurrency < rc.cfg.BatchSlots {
			appendFailure(&result.Failures, "turn %d reached concurrency %d, want all %d slots", turn, turnSummary.MaximumConcurrency, rc.cfg.BatchSlots)
		}
		if turnSummary.QueuedRequests < rc.cfg.BatchConversations-rc.cfg.BatchSlots {
			appendFailure(&result.Failures, "turn %d observed %d queued requests, want at least %d", turn, turnSummary.QueuedRequests, rc.cfg.BatchConversations-rc.cfg.BatchSlots)
		}
		if turn > 1 && turnSummary.MinimumCached <= 0 {
			appendFailure(&result.Failures, "turn %d reported no cached tokens for at least one conversation", turn)
		}
		if len(result.Failures) > 0 {
			break
		}
		conversations = updated
		rc.artifacts.logf("batch turn %02d: %d/%d complete prompt=%d cached=%d max_concurrency=%d queued=%d slowest=%.3fs",
			turn, turnSummary.Completed, rc.cfg.BatchConversations, turnSummary.MinimumPrompt,
			turnSummary.MinimumCached, turnSummary.MaximumConcurrency, turnSummary.QueuedRequests, turnSummary.SlowestSeconds)
	}

	if len(turns) == rc.cfg.BatchTurns && turns[len(turns)-1].MinimumPrompt < rc.cfg.BatchTargetTokens {
		appendFailure(&result.Failures, "final prompt reached %d tokens, want at least %d", turns[len(turns)-1].MinimumPrompt, rc.cfg.BatchTargetTokens)
	}
	result.Details["configuration"] = map[string]any{
		"slots": rc.cfg.BatchSlots, "conversations": rc.cfg.BatchConversations,
		"turns": rc.cfg.BatchTurns, "target_tokens": rc.cfg.BatchTargetTokens,
		"calibrated_turn_tokens": calibrated,
	}
	result.Details["turns"] = turns
	result.Details["requests"] = allRequests
	return result
}

func runBatchTurn(rc *runContext, model string, turn, records int, conversations [][]map[string]any) ([]batchRequestResult, [][]map[string]any) {
	count := len(conversations)
	results := make(chan batchRequestResult, count)
	updated := make([][]map[string]any, count)
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(count)
	for session := range count {
		go func() {
			userContent := batchRecordPrompt(session, turn, records)
			messages := append([]map[string]any{}, conversations[session]...)
			messages = append(messages, map[string]any{"role": "user", "content": userContent})
			body := map[string]any{
				"model": model, "stream": true, "stream_options": map[string]any{"include_usage": true},
				"temperature": 0, "seed": rc.cfg.Seed, "max_tokens": rc.cfg.BatchMaxTokens,
				"enable_thinking": false, "messages": messages,
			}
			ready.Done()
			<-start
			stream, err := rc.client.stream(rc, rc.scenario, body, nil)
			request := batchRequestResult{
				Session: session + 1, Turn: turn, TraceID: stream.TraceID, ResponseID: stream.ResponseID,
				ElapsedSeconds: seconds(stream.FinishedAt.Sub(stream.StartedAt)), PromptTokens: stream.Usage.PromptTokens,
				CachedTokens: stream.Usage.PromptTokensDetails.CachedTokens, CompletionTokens: stream.Usage.CompletionTokens,
				DraftTokens: stream.Usage.DraftTokens, DraftCoverage: stream.Usage.DraftCoverage,
				DraftDisableReason: stream.Usage.DraftDisableReason, FinishReason: stream.FinishReason,
				firstContentAt: stream.FirstContentAt, finishedAt: stream.FinishedAt,
			}
			if !stream.FirstContentAt.IsZero() {
				request.FirstContentSeconds = seconds(stream.FirstContentAt.Sub(stream.StartedAt))
			}
			if err != nil {
				request.Error = err.Error()
			} else if err := verifyBatchOutput(session, turn, stream.Content, stream.FinishReason); err != nil {
				request.Error = err.Error()
			} else {
				updated[session] = append(messages, map[string]any{"role": "assistant", "content": stream.Content})
			}
			results <- request
		}()
	}
	ready.Wait()
	close(start)
	requests := make([]batchRequestResult, count)
	for range count {
		request := <-results
		requests[request.Session-1] = request
	}
	return requests, updated
}

func summarizeBatchTurn(turn int, requests []batchRequestResult) batchTurnSummary {
	summary := batchTurnSummary{Turn: turn, MinimumPrompt: -1, MinimumCached: -1}
	var earliestFinish time.Time
	for _, request := range requests {
		if request.Error == "" {
			summary.Completed++
		}
		if summary.MinimumPrompt < 0 || request.PromptTokens < summary.MinimumPrompt {
			summary.MinimumPrompt = request.PromptTokens
		}
		if summary.MinimumCached < 0 || request.CachedTokens < summary.MinimumCached {
			summary.MinimumCached = request.CachedTokens
		}
		if request.ElapsedSeconds > summary.SlowestSeconds {
			summary.SlowestSeconds = request.ElapsedSeconds
		}
		if !request.finishedAt.IsZero() && (earliestFinish.IsZero() || request.finishedAt.Before(earliestFinish)) {
			earliestFinish = request.finishedAt
		}
	}
	for _, request := range requests {
		if !earliestFinish.IsZero() && !request.firstContentAt.IsZero() && !request.firstContentAt.Before(earliestFinish) {
			summary.QueuedRequests++
		}
	}
	summary.MaximumConcurrency = maximumBatchConcurrency(requests)
	return summary
}

func maximumBatchConcurrency(requests []batchRequestResult) int {
	type boundary struct {
		at    time.Time
		delta int
	}
	boundaries := make([]boundary, 0, len(requests)*2)
	for _, request := range requests {
		if request.firstContentAt.IsZero() || request.finishedAt.IsZero() {
			continue
		}
		boundaries = append(boundaries, boundary{at: request.firstContentAt, delta: 1}, boundary{at: request.finishedAt, delta: -1})
	}
	for left := 0; left < len(boundaries); left++ {
		for right := left + 1; right < len(boundaries); right++ {
			if boundaries[right].at.Before(boundaries[left].at) ||
				(boundaries[right].at.Equal(boundaries[left].at) && boundaries[right].delta < boundaries[left].delta) {
				boundaries[left], boundaries[right] = boundaries[right], boundaries[left]
			}
		}
	}
	current, maximum := 0, 0
	for _, boundary := range boundaries {
		current += boundary.delta
		maximum = max(maximum, current)
	}
	return maximum
}

func verifyBatchOutput(session, turn int, output, finishReason string) error {
	if output == "" {
		return fmt.Errorf("empty response")
	}
	expected := fmt.Sprintf("%s-TURN-%d", batchConversationKey(session), turn)
	markers := batchMarkerPattern.FindAllString(output, -1)
	count := 0
	for _, marker := range markers {
		if marker != expected {
			return fmt.Errorf("response contained foreign marker %s, want only %s", marker, expected)
		}
		count++
	}
	if count < batchCanaryRepetitions {
		return fmt.Errorf("response produced %d copies of %s, want %d; finish=%s", count, expected, batchCanaryRepetitions, finishReason)
	}
	return nil
}

func calibrateBatchRecords(rc *runContext, model string) (int, error) {
	low, high := 1, max(2, rc.cfg.BatchTokensPerTurn)
	for {
		count, err := rc.tokenCount(model, batchRecordPrompt(0, 1, high), false)
		if err != nil {
			return 0, err
		}
		if count >= rc.cfg.BatchTokensPerTurn {
			break
		}
		high *= 2
	}
	for low < high {
		middle := (low + high) / 2
		count, err := rc.tokenCount(model, batchRecordPrompt(0, 1, middle), false)
		if err != nil {
			return 0, err
		}
		if count < rc.cfg.BatchTokensPerTurn {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low, nil
}

func batchConversationKey(session int) string {
	return fmt.Sprintf("BATCH-SESSION-%02d", session+1)
}

func batchRecordPrompt(session, turn, records int) string {
	key := batchConversationKey(session)
	var prompt strings.Builder
	fmt.Fprintf(&prompt, "Conversation key: %s. This is turn %d.\n", key, turn)
	prompt.WriteString("Retain the conversation and read these compact operational records:\n")
	for record := range records {
		fmt.Fprintf(&prompt, "record %04d: latency budget retry owner archive signal metric policy.\n", record)
	}
	marker := fmt.Sprintf("%s-TURN-%d", key, turn)
	fmt.Fprintf(&prompt, "Reply by repeating %s exactly %d times, separated by spaces. Do not output anything else.", marker, batchCanaryRepetitions)
	return prompt.String()
}

func batchMessageText(messages []map[string]any) string {
	var values []string
	for _, message := range messages {
		values = append(values, fmt.Sprint(message["content"]))
	}
	return strings.Join(values, "\n")
}
