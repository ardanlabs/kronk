package model

import (
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

		s.span.End()
		e.freeSlotResources(s)
		s.reset()

		remaining := e.model.activeStreams.Add(-1)

		e.model.log(ctx, "batch-engine",
			"status", "slot-finished",
			"slot", slotID,
			"seq", seqID,
			"id", jobID,
			"active_streams", remaining,
		)
	}()

	var elapsed time.Duration
	if !s.startTime.IsZero() {
		elapsed = time.Since(s.startTime)
	}

	// Trim generated tokens from draft KV, keeping the cached prompt prefix
	// for incremental reuse on the next request.
	if e.model.draft != nil {
		trimPos := llama.Pos(len(e.model.draft.cachedTokens))
		if trimPos > 0 {
			llama.MemorySeqRm(e.model.draft.mem, s.seqID, trimPos, -1)
			e.model.log(ctx, "speculative", "status", "draft-kv-trimmed",
				"slot", slotID, "seq", seqID, "trim_pos", trimPos)
		} else {
			llama.MemorySeqRm(e.model.draft.mem, s.seqID, -1, -1)
			e.model.log(ctx, "speculative", "status", "draft-kv-cleared",
				"slot", slotID, "seq", seqID)
		}
	}

	// IMC dedicated slot mode: trim generated tokens but keep cached prefix.
	// Non-IMC mode: clear the entire sequence.
	if e.model.cfg.IncrementalCache && s.job.imcCacheHit {
		var trimPos llama.Pos

		e.model.cacheMu.RLock()
		if slotID < len(e.model.imcSlots) {
			trimPos = llama.Pos(e.model.imcSlots[slotID].totalTokensCached)
		}
		e.model.cacheMu.RUnlock()

		if trimPos > 0 {
			// Hybrid models: partial MemorySeqRm corrupts recurrent state
			// (DeltaNet/SSM). Use full clear + snapshot restore instead.
			if e.model.modelInfo.IsHybridModel {
				if len(s.imcSavedState) > 0 {
					e.model.decodeMu.Lock()
					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
					nRead := llama.StateSeqSetData(e.model.lctx, s.imcSavedState, s.seqID)
					e.model.decodeMu.Unlock()

					if nRead == 0 {
						e.model.log(ctx, "finish-slot", "status", "imc-hybrid-restore-failed",
							"slot", slotID, "seq", seqID, "trim_pos", trimPos,
							"snapshot_bytes", len(s.imcSavedState))

						// Guardrail: clear IMC metadata so the slot isn't
						// reused with a corrupt sequence.
						e.model.cacheMu.Lock()
						if slotID < len(e.model.imcSlots) {
							imcSlot := e.model.imcSlots[slotID]
							imcSlot.cachedMsgsHash = ""
							imcSlot.totalTokensCached = 0
							imcSlot.lastMsgIdxCached = 0
						}
						e.model.cacheMu.Unlock()
					} else {
						e.model.log(ctx, "finish-slot", "status", "imc-hybrid-restore",
							"slot", slotID, "seq", seqID, "trim_pos", trimPos,
							"snapshot_bytes", len(s.imcSavedState), "restored_bytes", nRead)
					}
				} else {
					// No snapshot available: full clear + invalidate metadata
					// to prevent reuse with corrupted recurrent state.
					e.model.log(ctx, "finish-slot", "status", "imc-hybrid-no-snapshot",
						"slot", slotID, "seq", seqID, "trim_pos", trimPos)

					llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)

					e.model.cacheMu.Lock()
					if slotID < len(e.model.imcSlots) {
						imcSlot := e.model.imcSlots[slotID]
						imcSlot.cachedMsgsHash = ""
						imcSlot.totalTokensCached = 0
						imcSlot.lastMsgIdxCached = 0
					}
					e.model.cacheMu.Unlock()
				}
			} else {
				llama.MemorySeqRm(e.model.mem, s.seqID, trimPos, -1)
				e.model.log(ctx, "finish-slot", "status", "imc-trim", "slot", slotID, "seq", seqID, "trim_pos", trimPos)
			}
		}
	} else {
		llama.MemorySeqRm(e.model.mem, s.seqID, -1, -1)
	}

	// Handle error case.
	if err != nil {
		usage := Usage{
			PromptTokens:     s.nPrompt,
			ReasoningTokens:  s.reasonTokens,
			CompletionTokens: s.completionTokens,
			OutputTokens:     s.reasonTokens + s.completionTokens,
			TotalTokens:      s.nPrompt + s.reasonTokens + s.completionTokens,
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
		PromptTokens:     s.nPrompt,
		ReasoningTokens:  s.reasonTokens,
		CompletionTokens: s.completionTokens,
		OutputTokens:     outputTokens,
		TotalTokens:      totalTokens,
		TokensPerSecond:  tokensPerSecond,
	}

	// Add span attributes and end span.
	s.span.SetAttributes(
		attribute.Int("prompt_tokens", s.nPrompt),
		attribute.Int("reasoning_tokens", s.reasonTokens),
		attribute.Int("completion_tokens", s.completionTokens),
		attribute.Int("output_tokens", outputTokens),
		attribute.Int("total_tokens", totalTokens),
		attribute.Float64("tokens_per_second", tokensPerSecond),
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

	e.model.log(ctx, "batch-engine", "status", "slot-finished", "slot", s.id, "id", s.job.id,
		"total_prompt", s.nPrompt, "output_tokens", outputTokens, "time", elapsed.String())
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
	if job.imcCacheHit && len(job.imcNewCacheTokens) > 0 {
		slotID := job.imcSlotID
		e.model.cacheMu.Lock()
		if slotID < len(e.model.imcSlots) {
			e.model.imcSlots[slotID].pending = false
		}
		e.model.cacheMu.Unlock()
		e.model.notifyIMCSlotAvailable()
	}

	close(job.ch)

	remaining := e.model.activeStreams.Add(-1)

	e.model.log(job.ctx, "batch-engine", "status", "job-failed", "id", job.id,
		"imc_slot", job.imcSlotID, "imc_seq", job.imcSeqID, "imc_cache_hit", job.imcCacheHit,
		"err", err, "active_streams", remaining)
}

func (e *batchEngine) freeSlotResources(s *slot) {
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
