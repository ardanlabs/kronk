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

	// IMC dedicated slot mode: trim generated tokens but keep cached prefix.
	// Non-IMC mode: clear the entire sequence.
	if e.model.cfg.IncrementalCache && s.job.imcID != "" {
		e.model.cacheMu.RLock()
		session, exists := e.model.imcSessions[s.job.imcID]
		var trimPos llama.Pos
		if exists {
			trimPos = llama.Pos(session.totalTokensCached)
		}
		e.model.cacheMu.RUnlock()

		if trimPos > 0 {
			llama.MemorySeqRm(e.model.mem, s.seqID, trimPos, -1)
			e.model.log(ctx, "finish-slot", "status", "imc-trim", "slot", slotID, "seq", seqID, "trim_pos", trimPos)
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
	if elapsed.Seconds() > 0 {
		tokensPerSecond = float64(outputTokens) / elapsed.Seconds()
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
