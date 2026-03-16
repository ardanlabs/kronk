package model

import (
	"context"
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

	if !s.skipKVCleanup {
		e.cleanupSlotKV(ctx, s)
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
			switch {
			case e.model.modelInfo.IsGPTModel:
				s.respToolCalls = parseGPTToolCall(content)

			default:
				s.respToolCalls = parseToolCall(content)
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

	// Send final response.
	returnPrompt := ""
	if s.job.params.ReturnPrompt {
		returnPrompt = s.job.prompt
	}

	e.model.sendFinalResponse(ctx, s.job.ch, s.job.id, s.job.object, 0, returnPrompt,
		&s.finalContent, &s.finalReasoning, s.respToolCalls, s.logprobsData, s.job.params.Stream, usage)
}

// cleanupSlotKV handles post-generation KV cache cleanup. For IMC slots,
// it trims or restores the cached state to preserve the conversation prefix.
// For non-IMC slots, it clears the entire sequence.
func (e *batchEngine) cleanupSlotKV(ctx context.Context, s *slot) {
	slotID := s.id
	seqID := s.seqID

	if s.job.imc == nil {
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()
		e.model.log(ctx, "finish-slot", "status", "seq-cleared", "slot", slotID, "seq", seqID)
		return
	}

	var trimPos llama.Pos

	if snap, ok := e.model.cache.SnapshotSlot(slotID); ok {
		trimPos = llama.Pos(snap.TotalTokensCached)
	}

	if trimPos <= 0 {
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
		e.model.decodeMu.Unlock()
		e.model.log(ctx, "finish-slot", "status", "imc-no-cached-pos-clear", "slot", slotID, "seq", seqID)
		return
	}

	switch e.model.modelInfo.Type {
	case ModelTypeHybrid:
		e.finishSlotHybrid(ctx, s, slotID, seqID, trimPos)

	case ModelTypeDense, ModelTypeMoE:
		e.model.decodeMu.Lock()
		llama.MemorySeqRm(e.model.mem, s.seqID, trimPos, -1)
		e.model.decodeMu.Unlock()
		e.model.log(ctx, "finish-slot", "status", "imc-trim", "slot", slotID, "seq", seqID, "trim_pos", trimPos)
	}
}

// failJob fails a job that was dequeued but never assigned to a slot. It sends
// an error response, ends the queue-wait span, closes the channel, clears any
// pending IMC reservation, and decrements activeStreams.
func (e *batchEngine) failJob(job *chatJob, err error) {
	e.model.sendErrorResponse(job.ctx, job.ch, job.id, job.object, 0, "", err, Usage{})

	if job.queueWaitSpan != nil {
		job.queueWaitSpan.End()
	}

	// Clear IMC pending reservation if this job reserved a slot.
	if imcJobReservedSlot(job.imc) {
		e.model.cache.ClearPending(job.imc.slotID)
	}

	close(job.ch)

	remaining := e.model.activeStreams.Add(-1)

	var imcSlot int
	var imcSeq llama.SeqId
	imcHit := job.imc != nil
	if imcHit {
		imcSlot = job.imc.slotID
		imcSeq = job.imc.seqID
	}

	e.model.log(job.ctx, "batch-engine", "status", "job-failed", "id", job.id,
		"imc_slot", imcSlot, "imc_seq", imcSeq, "imc_active", imcHit,
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
