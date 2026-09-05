package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// finishSlot completes a generation slot and sends the final response.
func (e *batchEngine) finishSlot(s *slot, err error) {
	if !s.active {
		return
	}
	if e.batchAssembling {
		e.batchReleased[s.id] = true
	}

	ctx := s.job.ctx
	jobID := s.job.id
	jobCh := s.job.ch
	slotID := s.id
	seqID := s.seqID
	nPrompt := s.nPrompt
	imcTokenPlan := s.job.imcTokenPlan
	imcMatchKind := s.job.imcMatchKind
	imcSessionID := s.job.imcSessionID
	imcActive := s.job.imcCacheHit
	imcSnapshotReused := s.job.imcSnapshotReused
	imcTailTokens := len(s.job.tailTokens)
	imcInputMessages := messageCount(s.job.d)
	imcStableInputTokens := len(s.job.imcNewCachedTokens)
	imcReusedTokens := s.job.reusedPromptTokens
	imcSystemBoundaryTokens := s.job.imcSystemBoundaryTokens
	mtpResumeSource := s.mtp.ResumeSource
	stageStarted := s.prefillStart

	var elapsed time.Duration

	defer func() {
		if s.prefillSpan != nil {
			s.prefillSpan.End()
			s.prefillSpan = nil
		}

		if s.tokenGenSpan != nil {
			s.tokenGenSpan.SetAttributes(
				attribute.Int("output_tokens", s.reasonTokens+s.completionTokens),
			)
			s.tokenGenSpan.End()
			s.tokenGenSpan = nil
		}

		outputTokens := s.reasonTokens + s.completionTokens
		draftTokens := s.specDraftedTotal
		draftAcceptedTokens := s.specAcceptedTotal
		draftCoveredTokens := s.specCoveredTotal
		disableReason := s.mtp.DisableReason

		s.span.End()
		e.freeSlotResources(s)
		s.reset()

		// Decrement activeStreams BEFORE close(jobCh). The model-level
		// activeStreams counter coordinates Model.Unload — closing the
		// channel before decrementing leaves a window where Unload could
		// race past the count. The pool-visible flake on a one-slot
		// pool (cap-evict-failed) is fixed at the outer kronk.Kronk
		// layer in concurrency.go (release before close); this inner
		// ordering keeps the per-model accounting consistent for the
		// same reason. Note: s.reset() above sets s.job = nil, so we
		// must close via the locally captured jobCh, not s.job.ch.
		remaining := e.model.activeStreams.Add(-1)
		metrics.AddPoolActiveStreams(e.model.modelInfo.ID, -1)

		var lifecycleElapsed time.Duration
		if !stageStarted.IsZero() {
			lifecycleElapsed = time.Since(stageStarted)
		}

		args := []any{
			"status", "slot-finished",
			"slot", slotID,
			"seq", seqID,
			"id", jobID,
			"total_prompt", nPrompt,
			"output_tokens", outputTokens,
			"imc_cache_mode", func() string {
				if imcTokenPlan {
					return "token-v2"
				}
				return "none"
			}(),
			"imc_cache_entry", imcSessionID,
			"imc_active", imcActive,
			"imc_cache_hit", imcSnapshotReused,
			"imc_match_kind", imcMatchKind,
			"imc_input_messages", imcInputMessages,
			"imc_input_tokens", nPrompt,
			"imc_stable_input_tokens", imcStableInputTokens,
			"imc_actual_reused_tokens", imcReusedTokens,
			"imc_system_boundary_tokens", imcSystemBoundaryTokens,
			"imc_tail_tokens", imcTailTokens,
			"elapsed", elapsed.String(),
			"active_streams", remaining,
		}

		// When a draft model is configured, always emit draft metrics so
		// the log schema stays stable for scrapers/dashboards even when
		// speculation was disabled mid-request (adaptive sizing returned 0
		// due to a collapsed acceptance EMA). Models without a draft
		// model omit the fields entirely.
		if e.model.draft != nil {
			switch {
			case disableReason != "":
				mtpResumeSource = "disabled"
			case mtpResumeSource == "":
				mtpResumeSource = "fresh-prefill"
			}
			var rate float64
			if draftTokens > 0 {
				rate = float64(draftAcceptedTokens) / float64(draftTokens)
			}
			var coverage float64
			if outputTokens > 0 {
				coverage = float64(draftCoveredTokens) / float64(outputTokens)
			}
			args = append(args,
				"draft_tokens", draftTokens,
				"draft_accepted_tokens", draftAcceptedTokens,
				"draft_acceptance_rate", fmt.Sprintf("%.2f", rate),
				"draft_coverage", fmt.Sprintf("%.2f", coverage),
				"draft_resume_source", mtpResumeSource,
			)
			if disableReason != "" {
				args = append(args, "draft_disable_reason", disableReason)
			}
		}

		e.model.log(ctx, "batch-engine", args...)
		e.model.log(ctx, "request-lifecycle",
			"stage", 4,
			"stage_name", "execute-in-slot",
			"status", lifecycleStatus(err),
			"id", jobID,
			"slot", slotID,
			"seq", seqID,
			"elapsed", lifecycleElapsed.String(),
			"err", err,
		)
		close(jobCh)
	}()

	if !s.startTime.IsZero() {
		elapsed = time.Since(s.startTime)
	}

	// Trim generated tokens from draft KV, keeping the cached prompt prefix
	// for incremental reuse on the next request.
	if e.model.draft != nil {
		_, sharedMTP := e.model.draft.(*sharedMTPDrafter)
		if !sharedMTP {
			trimPos := llama.Pos(len(s.draftCachedTokens))
			switch {
			case trimPos > 0:
				removed, rmErr := llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, trimPos, -1)
				switch {
				case rmErr != nil || !removed:
					llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, -1, -1)
					s.draftCachedTokens = s.draftCachedTokens[:0]
					e.model.log(ctx, "speculative", "status", "draft-kv-trim-fallback-clear",
						"slot", slotID, "seq", seqID, "trim_pos", trimPos, "err", rmErr)
				default:
					e.model.log(ctx, "speculative", "status", "draft-kv-trimmed",
						"slot", slotID, "seq", seqID, "trim_pos", trimPos)
				}
			default:
				llama.MemorySeqRm(e.model.draft.core().mem, s.seqID, -1, -1)
				e.model.log(ctx, "speculative", "status", "draft-kv-cleared",
					"slot", slotID, "seq", seqID)
			}
		}
	}

	// IMC: clear the entire sequence. The cached prefix KV state was
	// snapshotted into session.kvState in startSlot and will be restored
	// from its SessionStore on the next request. This applies to both text-only and
	// media sessions — StateSeqGetData captures raw KV bytes regardless
	// of whether they were produced by text tokens or media embeddings.
	//
	// Non-IMC: always clear.
	e.model.decodeMu.Lock()
	llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	e.model.decodeMu.Unlock()
	e.model.log(ctx, "finish-slot", "status", "seq-cleared", "slot", slotID, "seq", seqID)

	// Unbind the IMC session from this slot's KV sequence. The session
	// is now externalized in session.kvState and not resident in any
	// model KV sequence, so the defensive
	// KV-pressure eviction path should no longer issue MemorySeqRm
	// against this session's seqID.
	if s.job.imcSession != nil {
		outputTokens := s.reasonTokens + s.completionTokens
		e.model.imcRecordRequestUsage(s.job.imcSession, s.job.imcUsageVersion, messageCount(s.job.d), s.nPrompt, outputTokens, int(s.nPast))
		e.model.cacheMu.Lock()
		// A newly published snapshot may already be serving another slot.
		// Only unbind the sequence this request actually owned.
		if s.job.imcSession.seqID == s.seqID {
			s.job.imcSession.seqID = imcSeqIDUnbound
		}
		e.model.cacheMu.Unlock()
	}
	if s.job.hasIMCReservation() {
		e.model.imcReleaseReservation(s.job.imcSessionID)
	}

	outputTokens := s.reasonTokens + s.completionTokens
	totalTokens := s.nPrompt + outputTokens
	var tokensPerSecond float64
	if elapsed.Seconds() > 0 && outputTokens > 1 {
		tokensPerSecond = float64(outputTokens-1) / elapsed.Seconds()
	}
	usage := Usage{
		PromptTokens: s.nPrompt,
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens: s.reusedPromptTokens,
		},
		CompletionTokens: outputTokens,
		CompletionTokensDetails: CompletionTokensDetails{
			ReasoningTokens: s.reasonTokens,
		},
		TotalTokens:         totalTokens,
		TokensPerSecond:     tokensPerSecond,
		TimeToFirstTokenMS:  float64(s.ttft.Microseconds()) / 1000.0,
		DraftTokens:         s.specDraftedTotal,
		DraftAcceptedTokens: s.specAcceptedTotal,
		DraftDisableReason:  s.mtp.DisableReason,
	}
	if usage.DraftTokens > 0 {
		usage.DraftAcceptanceRate = float64(usage.DraftAcceptedTokens) / float64(usage.DraftTokens)
	}
	if outputTokens > 0 && e.model.draft != nil {
		usage.DraftCoverage = float64(s.specCoveredTotal) / float64(outputTokens)
	}

	// Handle error case.
	if err != nil {
		status := "error"
		class := "active-slot"
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
			status = "cancel"
			class = "context-cancelled"
		}
		s.span.RecordError(err)
		s.span.SetAttributes(
			attribute.String("request_status", status),
			attribute.Int("prompt_tokens", s.nPrompt),
			attribute.Int("reasoning_tokens", s.reasonTokens),
			attribute.Int("completion_tokens", s.completionTokens),
			attribute.Int("output_tokens", outputTokens),
			attribute.Int("total_tokens", usage.TotalTokens),
			attribute.Float64("tokens_per_second", tokensPerSecond),
			attribute.Int("draft_tokens", s.specDraftedTotal),
			attribute.Int("draft_accepted_tokens", s.specAcceptedTotal),
			attribute.Int("draft_covered_tokens", s.specCoveredTotal),
		)
		metrics.AddChatRequest(e.model.modelInfo.ID, status)
		metrics.AddChatError(e.model.modelInfo.ID, class)
		if !s.job.requestStart.IsZero() {
			metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(s.job.requestStart))
		}

		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, err, usage)

		return
	}

	if s.stopGate != nil && s.stopSource != "request-stop" && s.stopSource != "parser-eog" {
		pieces := s.stopGate.flush()
		for i, piece := range pieces {
			logprobIndex := appendStopPieceLogprobs(s, piece)
			outcome := e.processDecodedPiece(s, piece, logprobIndex, true)
			if outcome.parserEOG {
				reconcileParserEOGRemainder(s, pieces[i+1:])
				break
			}
			if outcome.err != nil {
				outputTokens := s.reasonTokens + s.completionTokens
				usage := Usage{PromptTokens: s.nPrompt, CompletionTokens: outputTokens, TotalTokens: s.nPrompt + outputTokens}
				e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, outcome.err, usage)
				return
			}
		}
	}

	if flusher, ok := s.stateMachine.(StateMachineFlusher); ok {
		e.flushAllStateMachine(s, flusher)
	}

	// Flush any remaining buffered UTF-8 bytes into the final accumulators.
	// Only emit complete codepoints; drop any trailing incomplete sequence
	// to avoid injecting replacement characters into the final response.
	if len(s.utf8Buf) > 0 {
		complete, _ := extractCompleteUTF8(s.utf8Buf)
		if len(complete) > 0 {
			leftover := string(complete)
			switch {
			case s.reasonFlag > 0:
				s.finalReasoning.WriteString(leftover)
			case s.toolFlag > 0:
				s.finalTooling.WriteString(leftover)
			default:
				s.finalContent.WriteString(leftover)
			}
		}
		s.utf8Buf = s.utf8Buf[:0]
	}

	// Process tool calls if any.
	var toolCallErr error
	var lengthTerminatedToolContent string
	var lengthTerminatedToolOutput bool
	var bufferedToolBytes int
	if s.finalTooling.Len() > 0 {
		content := strings.TrimSuffix(s.finalTooling.String(), "\n")
		bufferedToolBytes = len(content)
		if len(content) > 0 {

			// Log the decoded model output captured before parser classification
			// so tool call issues can be debugged. Only logged when insecure
			// logging is enabled.
			if e.model.cfg.InsecureLogging() {
				rawContent := s.rawOutput.String()
				if rawContent == "" {
					rawContent = content
				}
				e.model.log(ctx, "tool-call", "status", "raw-model-output",
					"bytes", len(rawContent), "content", rawContent,
					"buffered_content", content)
			}

			lengthTerminatedToolContent, lengthTerminatedToolOutput = retainLengthTerminatedToolOutput(s.finishReason, &s.finalContent, &s.respToolCalls)
			if lengthTerminatedToolOutput {
				e.model.log(ctx, "tool-call",
					"status", "max-tokens-output-replaced",
					"bytes", len(lengthTerminatedToolContent),
					"stream", s.job.params.Stream,
					"parser", e.model.parser.Name())
			} else {
				if parser, ok := e.model.parser.(ToolCallSchemaParser); ok {
					tools, _ := s.job.d["tools"].([]D)
					s.respToolCalls = parser.ToolCallWithSchema(ctx, e.model.log, content, tools)
				} else {
					s.respToolCalls = e.model.parser.ToolCall(ctx, e.model.log, content)
				}

				// Validate parsed tool call arguments produce valid JSON.
				for i, tc := range s.respToolCalls {
					if tc.Status != 0 {
						e.model.log(ctx, "tool-call", "status", "parse-error",
							"index", i, "func", tc.Function.Name,
							"error", tc.Error, "raw", tc.Raw)
						if toolCallErr == nil {
							toolCallErr = fmt.Errorf("model emitted an invalid tool call at index %d: %s", i, tc.Error)
						}
						continue
					}

					if _, err := json.Marshal(map[string]any(tc.Function.Arguments)); err != nil {
						e.model.log(ctx, "tool-call", "status", "invalid-args",
							"index", i, "func", tc.Function.Name,
							"error", err)
						s.respToolCalls[i].Status = 2
						s.respToolCalls[i].Error = err.Error()
						if toolCallErr == nil {
							toolCallErr = fmt.Errorf("model emitted invalid tool call arguments at index %d: %w", i, err)
						}
					}
				}
			}
		}
	}

	var startedToolCalls []ResponseToolCallDelta
	if s.job.params.Stream {
		if streamer, ok := s.stateMachine.(ToolCallDeltaStreamer); ok {
			startedToolCalls = streamer.StartedToolCalls()
		}
	}

	var invalidToolCallContent string
	if toolCallErr != nil && s.stopSource == "request-stop" {
		valid := s.respToolCalls[:0]
		for _, toolCall := range s.respToolCalls {
			if toolCall.Status == 0 {
				valid = append(valid, toolCall)
			}
		}
		s.respToolCalls = valid
	} else if toolCallErr != nil {
		var recovered bool
		invalidToolCallContent, recovered = recoverInvalidToolCallOutput(s.job.params.Stream, startedToolCalls, &s.finalContent, &s.respToolCalls)
		if recovered {
			e.model.log(ctx, "tool-call", "status", "parse-error-recovered",
				"parser", e.model.parser.Name(), "remaining", len(s.respToolCalls))
			toolCallErr = nil
		}
	}

	// Add span attributes and end span.
	s.span.SetAttributes(
		attribute.Int("prompt_tokens", s.nPrompt),
		attribute.Int("reasoning_tokens", s.reasonTokens),
		attribute.Int("completion_tokens", s.completionTokens),
		attribute.Int("output_tokens", outputTokens),
		attribute.Int("total_tokens", totalTokens),
		attribute.Float64("tokens_per_second", tokensPerSecond),
		attribute.Int("draft_tokens", s.specDraftedTotal),
		attribute.Int("draft_accepted_tokens", s.specAcceptedTotal),
		attribute.Int("draft_covered_tokens", s.specCoveredTotal),
	)
	if toolCallErr != nil && s.finishReason != FinishReasonLength && s.stopSource != "request-stop" {
		err = toolCallErr
		s.span.RecordError(err)
		s.span.SetAttributes(attribute.String("request_status", "error"))
		metrics.AddChatRequest(e.model.modelInfo.ID, "error")
		metrics.AddChatError(e.model.modelInfo.ID, "tool-call-parse")
		if !s.job.requestStart.IsZero() {
			metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(s.job.requestStart))
		}
		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, err, usage)
		return
	}

	// Send final response.
	if s.stopSource == "" {
		s.stopSource = "unknown"
	}
	var deliveryErr error
	if s.job.params.Stream && lengthTerminatedToolContent != "" {
		deliveryErr = e.model.sendDeltaResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, lengthTerminatedToolContent, ChannelAnswer, s.reasonTokens, outputTokens, nil)
	}
	if s.job.params.Stream && invalidToolCallContent != "" && deliveryErr == nil {
		deliveryErr = e.model.sendDeltaResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, invalidToolCallContent, ChannelAnswer, s.reasonTokens, outputTokens, nil)
	}
	var terminalToolCallDeltas []ResponseToolCallDelta
	if s.job.params.Stream {
		terminalToolCallDeltas = reconcileStartedToolCalls(s.respToolCalls, startedToolCalls)
	}
	finalChannel := slotChannel(s)
	if lengthTerminatedToolOutput || invalidToolCallContent != "" {
		finalChannel = ChannelAnswer
	}
	if deliveryErr == nil {
		deliveryErr = e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, 0,
			&s.finalContent, &s.finalReasoning, s.respToolCalls, terminalToolCallDeltas, s.logprobsData, s.finishReason, s.stopSource, finalChannel, bufferedToolBytes, s.job.params.Stream, !s.job.params.Stream || s.job.params.IncludeUsage, usage)
	}
	if deliveryErr != nil {
		err = deliveryErr
		status := "error"
		class := "terminal-delivery"
		if errors.Is(deliveryErr, context.Canceled) || errors.Is(deliveryErr, context.DeadlineExceeded) {
			status = "cancel"
			class = "context-cancelled"
		}
		s.span.RecordError(deliveryErr)
		s.span.SetAttributes(attribute.String("request_status", status))
		metrics.AddChatRequest(e.model.modelInfo.ID, status)
		metrics.AddChatError(e.model.modelInfo.ID, class)
		if !s.job.requestStart.IsZero() {
			metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(s.job.requestStart))
		}
		return
	}

	s.span.SetAttributes(attribute.String("request_status", "ok"))
	metrics.AddChatCompletionsUsage(e.model.modelInfo.ID, s.nPrompt, s.reasonTokens, s.completionTokens, outputTokens, totalTokens, tokensPerSecond)
	metrics.AddChatRequest(e.model.modelInfo.ID, "ok")
	if !s.job.requestStart.IsZero() {
		metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(s.job.requestStart))
	}
}

const lengthTerminatedToolMessage = "Response truncated before completion."

const invalidToolCallMessage = "The model produced an invalid tool call. Please try again."

func retainLengthTerminatedToolOutput(finishReason string, finalContent *strings.Builder, respToolCalls *[]ResponseToolCall) (string, bool) {
	if finishReason != FinishReasonLength {
		return "", false
	}

	finalContent.WriteString(lengthTerminatedToolMessage)
	*respToolCalls = nil
	return lengthTerminatedToolMessage, true
}

func recoverInvalidToolCallOutput(streaming bool, started []ResponseToolCallDelta, finalContent *strings.Builder, respToolCalls *[]ResponseToolCall) (string, bool) {
	if streaming && len(started) > 0 {
		return "", false
	}

	valid := (*respToolCalls)[:0]
	for _, toolCall := range *respToolCalls {
		if toolCall.Status == 0 {
			valid = append(valid, toolCall)
		}
	}
	*respToolCalls = valid
	if len(valid) > 0 {
		return "", true
	}

	delta := invalidToolCallMessage
	if finalContent.Len() > 0 {
		delta = "\n\n" + delta
	}
	finalContent.WriteString(delta)
	return delta, true
}

func reconcileStartedToolCalls(toolCalls []ResponseToolCall, started []ResponseToolCallDelta) []ResponseToolCallDelta {
	if len(toolCalls) == 0 {
		return nil
	}

	terminal := make([]ResponseToolCallDelta, len(toolCalls))
	matchedStarts := make([]bool, len(started))
	nextUnannouncedIndex := 0
	for _, start := range started {
		nextUnannouncedIndex = max(nextUnannouncedIndex, start.Index+1)
	}

	for i := range toolCalls {
		arguments := "{}"
		if toolCalls[i].Function.Arguments != nil {
			data, err := json.Marshal(map[string]any(toolCalls[i].Function.Arguments))
			if err != nil {
				return nil
			}
			arguments = string(data)
		}

		terminal[i] = ResponseToolCallDelta{
			ID:    toolCalls[i].ID,
			Index: toolCalls[i].Index,
			Type:  toolCalls[i].Type,
			Function: ResponseToolCallDeltaFunction{
				Name:      toolCalls[i].Function.Name,
				Arguments: arguments,
			},
		}

		startAt := -1
		for j := range started {
			if !matchedStarts[j] && toolCalls[i].Function.Name == started[j].Function.Name {
				startAt = j
				break
			}
		}
		if startAt == -1 {
			toolCalls[i].Index = nextUnannouncedIndex
			terminal[i].Index = nextUnannouncedIndex
			nextUnannouncedIndex++
			continue
		}

		matchedStarts[startAt] = true
		if toolCalls[i].Status == 0 {
			toolCalls[i].ID = started[startAt].ID
			toolCalls[i].Index = started[startAt].Index
		}
		terminal[i].ID = ""
		terminal[i].Index = started[startAt].Index
		terminal[i].Type = ""
		terminal[i].Function.Name = ""
	}

	return terminal
}

func (e *batchEngine) flushAllStateMachine(s *slot, flusher StateMachineFlusher) {
	for result := flusher.Flush(); result != (Result{}); result = flusher.Flush() {
		e.flushStateMachine(s, result)
	}
}

// flushStateMachine preserves model output held by parser lookahead when
// generation ends before the parser sees a closing delimiter.
func (e *batchEngine) flushStateMachine(s *slot, result Result) {
	if result.Content == "" {
		return
	}
	if s.suppressTools && result.Channel == ChannelTool {
		result.Channel = ChannelAnswer
	}

	outputTokens := s.reasonTokens + s.completionTokens

	updateSlotChannel(s, result.Channel)
	if err := e.retainAndStreamResult(s, result, outputTokens, nil, -1); err != nil {
		e.model.log(s.job.ctx, "parser-flush", "status", "delta-failed", "err", err)
	}
}

// failJob fails a job that was dequeued but never assigned to a slot. It sends
// an error response, ends the queue-wait span, closes the channel, clears any
// held IMC reservation, and decrements activeStreams.
func (e *batchEngine) failJob(job *chatJob, err error) {
	if job.queueWaitSpan != nil {
		job.queueWaitSpan.RecordError(err)
		job.queueWaitSpan.End()
	}
	if !job.queuedAt.IsZero() {
		metrics.ObserveChatQueueWait(e.model.modelInfo.ID, time.Since(job.queuedAt))
	}

	status := "error"
	class := "fail-job"
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		status = "cancel"
		class = "context-cancelled"
	}
	metrics.AddChatRequest(e.model.modelInfo.ID, status)
	metrics.AddChatError(e.model.modelInfo.ID, class)
	if !job.requestStart.IsZero() {
		metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(job.requestStart))
	}

	// Release the IMC reservation if this job reserved a session.
	if job.hasIMCReservation() {
		e.model.imcReleaseReservation(job.imcSessionID)
	}
	job.releaseIMCSystemCache(e.model)

	// Decrement activeStreams BEFORE close(job.ch). See finishSlot's
	// defer for the full rationale: closing first leaves a race window
	// where the next sequential request can hit ErrServerBusy while
	// this stream's count is still in flight.
	remaining := e.model.activeStreams.Add(-1)
	metrics.AddPoolActiveStreams(e.model.modelInfo.ID, -1)

	e.model.log(job.ctx, "request-lifecycle",
		"stage", 3,
		"stage_name", "schedule-job",
		"status", lifecycleStatus(err),
		"id", job.id,
		"elapsed", time.Since(job.queuedAt).String(),
		"err", err,
	)
	e.model.log(job.ctx, "batch-engine", "status", "job-failed", "id", job.id,
		"imc_cache_entry", job.imcSessionID, "imc_active", job.imcCacheHit,
		"imc_cache_hit", job.imcSnapshotReused,
		"err", err, "active_streams", remaining)

	e.model.sendErrorResponse(job.ctx, job.ch, job.id, job.object, 0, err, Usage{})
	close(job.ch)
}

func (e *batchEngine) freeSlotResources(s *slot) {
	// Unregister the per-slot draft sampler from the draft context before
	// freeing it, to prevent a dangling pointer in the context's sampler map.
	if s.draftSampler != 0 && e.model.draft != nil {
		draft := e.model.draft.core()
		if draft.registeredSampler == s.draftSampler {
			llama.SetSampler(draft.lctx, draft.registeredSeqID, 0)
			draft.registeredSampler = 0
		}
	}

	if s.sampler != 0 {
		llama.SamplerFree(s.sampler)
		s.sampler = 0
	}

	if s.grammarSampler != nil {
		s.grammarSampler.Free()
		s.grammarSampler = nil
	}

	// Free MTMD resources.
	if s.inputChunks != 0 {
		mtmd.InputChunksFree(s.inputChunks)
		s.inputChunks = 0
	}

	for _, b := range s.bitmaps {
		if b != 0 {
			mtmd.BitmapFree(b)
		}
	}
	s.bitmaps = nil

	// Free the per-request mtmd context. This is created on demand in
	// startSlot for media-bearing requests and lives only for the
	// duration of one request, so any internal state mtmd accumulates
	// (image_tokens, output buffer, bitmap registry, vision/audio
	// support flags) cannot bleed into subsequent requests.
	if s.mtmdCtx != 0 {
		mtmd.Free(s.mtmdCtx)
		s.mtmdCtx = 0
	}
}
