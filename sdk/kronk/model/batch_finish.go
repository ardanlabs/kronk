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

// finishSlot completes a slot and sends the final response.
func (e *batchEngine) finishSlot(s *slot, err error) {
	if !s.active {
		return
	}

	ctx := s.job.ctx
	jobID := s.job.id
	slotID := s.id
	seqID := s.seqID
	nPrompt := s.nPrompt

	var elapsed time.Duration

	defer func() {
		close(s.job.ch)

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

		s.span.End()
		e.freeSlotResources(s)
		s.reset()

		remaining := e.model.activeStreams.Add(-1)

		args := []any{
			"status", "slot-finished",
			"slot", slotID,
			"seq", seqID,
			"id", jobID,
			"total_prompt", nPrompt,
			"output_tokens", outputTokens,
			"time", elapsed.String(),
			"active_streams", remaining,
		}

		if draftTokens > 0 {
			rate := float64(draftAcceptedTokens) / float64(draftTokens)
			args = append(args,
				"draft_tokens", draftTokens,
				"draft_accepted_tokens", draftAcceptedTokens,
				"draft_acceptance_rate", fmt.Sprintf("%.2f", rate),
			)
		}

		e.model.log(ctx, "batch-engine", args...)
	}()

	if !s.startTime.IsZero() {
		elapsed = time.Since(s.startTime)
	}

	// Trim generated tokens from draft KV, keeping the cached prompt prefix
	// for incremental reuse on the next request.
	if e.model.draft != nil {
		trimPos := llama.Pos(len(s.draftCachedTokens))
		switch {
		case trimPos > 0:
			llama.MemorySeqRm(e.model.draft.mem, s.seqID, trimPos, -1)
			e.model.log(ctx, "speculative", "status", "draft-kv-trimmed",
				"slot", slotID, "seq", seqID, "trim_pos", trimPos)
		default:
			llama.MemorySeqRm(e.model.draft.mem, s.seqID, -1, -1)
			e.model.log(ctx, "speculative", "status", "draft-kv-cleared",
				"slot", slotID, "seq", seqID)
		}
	}

	// IMC: clear the entire sequence. The cached prefix KV state was
	// snapshotted into session.kvState in startSlot and will be restored
	// from RAM on the next request. This applies to both text-only and
	// media sessions — StateSeqGetData captures raw KV bytes regardless
	// of whether they were produced by text tokens or media embeddings.
	//
	// Non-IMC: always clear.
	switch {
	default:
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()
		e.model.log(ctx, "finish-slot", "status", "seq-cleared", "slot", slotID, "seq", seqID)
	}

	// Handle error case.
	if err != nil {
		outputTokens := s.reasonTokens + s.completionTokens

		var tokensPerSecond float64
		if elapsed.Seconds() > 0 && outputTokens > 1 {
			tokensPerSecond = float64(outputTokens-1) / elapsed.Seconds()
		}

		usage := Usage{
			PromptTokens:        s.nPrompt,
			ReasoningTokens:     s.reasonTokens,
			CompletionTokens:    s.completionTokens,
			OutputTokens:        outputTokens,
			TotalTokens:         s.nPrompt + outputTokens,
			TokensPerSecond:     tokensPerSecond,
			TimeToFirstTokenMS:  float64(s.ttft.Microseconds()) / 1000.0,
			DraftTokens:         s.specDraftedTotal,
			DraftAcceptedTokens: s.specAcceptedTotal,
		}

		if usage.DraftTokens > 0 {
			usage.DraftAcceptanceRate = float64(usage.DraftAcceptedTokens) / float64(usage.DraftTokens)
		}

		e.model.sendErrorResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, "", err, usage)

		return
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

	// Process tool calls if any. Token counts are already tracked
	// per-token in processSlotToken, so no re-tokenization needed.
	if s.toolFlag > 0 {
		content := strings.TrimSuffix(s.finalTooling.String(), "\n")
		if len(content) > 0 {

			// Log the raw model output before parsing so tool call issues
			// can be debugged. Only logged when insecure logging is enabled.
			if e.model.cfg.InsecureLogging() {
				e.model.log(ctx, "tool-call", "status", "raw-model-output",
					"bytes", len(content), "content", content)
			}

			switch {
			case e.model.modelInfo.IsGPTModel:
				s.respToolCalls = s.proc.parseGPTToolCall(content)

			default:
				s.respToolCalls = s.proc.parseToolCall(content)
			}

			// Validate parsed tool call arguments produce valid JSON.
			for i, tc := range s.respToolCalls {
				if tc.Status != 0 {
					e.model.log(ctx, "tool-call", "status", "parse-error",
						"index", i, "func", tc.Function.Name,
						"error", tc.Error, "raw", tc.Raw)
					continue
				}

				argsJSON, err := json.Marshal(map[string]any(tc.Function.Arguments))
				if err != nil {
					e.model.log(ctx, "tool-call", "status", "invalid-args",
						"index", i, "func", tc.Function.Name,
						"error", err)
				} else {
					var check map[string]any
					if err := json.Unmarshal(argsJSON, &check); err != nil {
						e.model.log(ctx, "tool-call", "status", "invalid-args-json",
							"index", i, "func", tc.Function.Name,
							"error", err, "json", string(argsJSON))
					}
				}
			}
		}
	}

	// Calculate final metrics.
	outputTokens := s.reasonTokens + s.completionTokens
	totalTokens := s.nPrompt + outputTokens

	var tokensPerSecond float64
	if elapsed.Seconds() > 0 && outputTokens > 1 {
		tokensPerSecond = float64(outputTokens-1) / elapsed.Seconds()
	}

	usage := Usage{
		PromptTokens:        s.nPrompt,
		ReasoningTokens:     s.reasonTokens,
		CompletionTokens:    s.completionTokens,
		OutputTokens:        outputTokens,
		TotalTokens:         totalTokens,
		TokensPerSecond:     tokensPerSecond,
		TimeToFirstTokenMS:  float64(s.ttft.Microseconds()) / 1000.0,
		DraftTokens:         s.specDraftedTotal,
		DraftAcceptedTokens: s.specAcceptedTotal,
	}

	if usage.DraftTokens > 0 {
		usage.DraftAcceptanceRate = float64(usage.DraftAcceptedTokens) / float64(usage.DraftTokens)
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
	)

	// Add metrics.
	metrics.AddChatCompletionsUsage(e.model.modelInfo.ID, s.nPrompt, s.reasonTokens, s.completionTokens, outputTokens, totalTokens, tokensPerSecond)
	metrics.AddChatRequest(e.model.modelInfo.ID, "ok")
	if !s.job.requestStart.IsZero() {
		metrics.ObserveChatRequestDuration(e.model.modelInfo.ID, time.Since(s.job.requestStart))
	}

	// Send final response.
	returnPrompt := ""
	if s.job.params.ReturnPrompt {
		returnPrompt = s.job.prompt
	}

	e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, returnPrompt,
		&s.finalContent, &s.finalReasoning, s.respToolCalls, s.logprobsData, s.job.params.Stream, usage)
}

// failJob fails a job that was dequeued but never assigned to a slot. It sends
// an error response, ends the queue-wait span, closes the channel, clears any
// pending IMC reservation, and decrements activeStreams.
func (e *batchEngine) failJob(job *chatJob, err error) {
	e.model.sendErrorResponse(job.ctx, job.ch, job.id, job.object, 0, "", err, Usage{})

	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
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

	// Clear IMC pending reservation if this job reserved a slot.
	if job.imcCacheHit && (len(job.imcNewCacheTokens) > 0 || job.imcMediaBuild) {
		e.model.imcClearPending(job.imcSlotID)
	}

	close(job.ch)

	remaining := e.model.activeStreams.Add(-1)

	e.model.log(job.ctx, "batch-engine", "status", "job-failed", "id", job.id,
		"imc_slot", job.imcSlotID, "imc_cache_hit", job.imcCacheHit,
		"err", err, "active_streams", remaining)
}

func (e *batchEngine) freeSlotResources(s *slot) {
	// Unregister the per-slot draft sampler from the draft context before
	// freeing it, to prevent a dangling pointer in the context's sampler map.
	if s.draftSampler != 0 && e.model.draft != nil {
		draft := e.model.draft
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

	// Free mtmdCtx from the job if present.
	if s.job != nil && s.job.mtmdCtx != 0 {
		mtmd.Free(s.job.mtmdCtx)
		s.job.mtmdCtx = 0
	}
}
