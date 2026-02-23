package model

import (
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// processSlotToken samples and processes a token for a slot.
func (e *batchEngine) processSlotToken(s *slot, buf []byte) {
	// Sample the next token. If grammar is active, use grammar-aware sampling.
	var token llama.Token
	switch {
	case s.grammarSampler != nil:
		token = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, s.iBatch)

	default:
		token = llama.SamplerSample(s.sampler, e.model.lctx, s.iBatch)
	}

	e.handleSampledToken(s, token, s.iBatch, buf)
}

// handleSampledToken processes a sampled token through the full pipeline:
// logprobs extraction, grammar/sampler acceptance, EOG check, state machine,
// streaming, and token counting. Used by both processSlotToken and sampleFirstToken.
func (e *batchEngine) handleSampledToken(s *slot, token llama.Token, iBatch int32, buf []byte) {
	// Extract logprobs BEFORE accepting - Accept modifies sampler state.
	// Reset currentLogprob each token; it's used for streaming.
	s.currentLogprob = nil
	if s.job.params.Logprobs {
		logprob, err := extractLogprobs(e.model.lctx, e.model.vocab, token, iBatch, s.job.params.TopLogprobs, buf)
		switch {
		case err != nil:
			e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())

		case logprob != nil:
			s.currentLogprob = logprob
			s.logprobsData = append(s.logprobsData, *logprob)
		}
	}

	// Accept token on both samplers. Grammar sampler is accepted separately
	// to avoid the crash that occurs when grammar is in the chain.
	if s.grammarSampler != nil {
		s.grammarSampler.Accept(token)
	}

	llama.SamplerAccept(s.sampler, token)

	// Check for end of generation.
	if llama.VocabIsEOG(e.model.vocab, token) {
		e.finishSlot(s, nil)
		return
	}

	// Convert token to text, buffering partial UTF-8 codepoints.
	l := llama.TokenToPiece(e.model.vocab, token, buf, 0, true)

	s.utf8Buf = append(s.utf8Buf, buf[:l]...)

	complete, remainder := extractCompleteUTF8(s.utf8Buf)

	// Convert to string BEFORE mutating the buffer. The complete slice
	// shares the same backing array as s.utf8Buf, so we must copy via
	// string() first to avoid corruption.
	var content string
	if len(complete) > 0 {
		content = string(complete)
	}

	switch {
	case len(remainder) > 0:
		s.utf8Buf = append(s.utf8Buf[:0], remainder...)
	default:
		s.utf8Buf = s.utf8Buf[:0]
	}

	s.sampled = token

	if !s.prefillDone {
		s.prefillDone = true
		s.startTime = time.Now() // Start TPS clock after prefill, when first output token is generated

		// Record TTFT and end the prefill span.
		var ttft time.Duration
		if !s.prefillStart.IsZero() {
			ttft = time.Since(s.prefillStart)
		}
		s.ttft = ttft
		metrics.AddTimeToFirstToken(e.model.modelInfo.ID, ttft)

		if s.prefillSpan != nil {
			if s.prefillSpan.IsRecording() {
				s.prefillSpan.SetAttributes(attribute.String("ttft", ttft.String()))
			}
			s.prefillSpan.End()
			s.prefillSpan = nil
		}

		// Start token generation span.
		_, s.tokenGenSpan = otel.AddSpan(s.job.ctx, "token-generation",
			attribute.Int("slot", s.id),
		)
	}

	// If no complete UTF-8 codepoints are ready, count the token using the
	// current flags (partial bytes can't trigger a state transition) and skip
	// the processor and streaming.
	if len(content) == 0 {
		switch {
		case s.reasonFlag > 0:
			s.reasonTokens++
		default:
			s.completionTokens++
		}

		outputTokens := s.reasonTokens + s.completionTokens

		if outputTokens >= s.job.params.MaxTokens {
			e.finishSlot(s, nil)
			return
		}

		s.iBatch = -1
		return
	}

	// Process through the state machine.
	isGPT := e.model.modelInfo.IsGPTModel
	var resp response
	var eog bool

	switch isGPT {
	case true:
		resp, eog = s.proc.stepGPT(content)

	default:
		resp, eog = s.proc.stepStandard(content)
	}

	if eog {
		e.finishSlot(s, nil)
		return
	}

	// Update flags based on response status.
	switch resp.status {
	case statusReasoning:
		s.reasonFlag++
		s.completionFlag = 0
		s.toolFlag = 0

	case statusCompletion:
		s.completionFlag++
		s.reasonFlag = 0
		s.toolFlag = 0

	case statusTooling:
		s.toolFlag++
		s.reasonFlag = 0
		s.completionFlag = 0

	default:
		// No streamable content (statusNone) - skip without counting.
		// This happens for control tokens like <|end|> which shouldn't be counted.
		s.iBatch = -1
		return
	}

	// Count the token after the state machine has updated the flags so
	// that attribution (reasoning vs completion) reflects the actual
	// section this token belongs to.
	switch {
	case s.reasonFlag > 0:
		s.reasonTokens++
	default:
		s.completionTokens++
	}

	outputTokens := s.reasonTokens + s.completionTokens

	if outputTokens >= s.job.params.MaxTokens {
		e.finishSlot(s, nil)
		return
	}

	// Store content for final response.
	switch {
	case s.reasonFlag > 0:
		s.finalReasoning.WriteString(resp.content)

	case s.toolFlag > 0:
		s.finalTooling.WriteString(resp.content)

	default:
		s.finalContent.WriteString(resp.content)
	}

	// Stream response if not tooling.
	if s.toolFlag == 0 {
		// Skip unnecessary CRLF at mode transitions.
		if e.model.isUnncessaryCRLF(s.reasonFlag, s.completionFlag, resp.content) {
			s.iBatch = -1
			return
		}

		// Per OpenAI spec, usage is only sent in the final response, not deltas.
		err := e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, "", resp.content, s.reasonFlag, outputTokens, s.currentLogprob)
		if err != nil {
			e.finishSlot(s, err)
			return
		}
	}

	s.iBatch = -1
}

// sampleFirstToken samples the first output token after prefill completes.
// This is called when the last chunk used a separate decode path (M-RoPE text
// or image embeddings) and nothing was added to the shared batch.
// Returns false if slot finished (EOG or error), true otherwise.
func (e *batchEngine) sampleFirstToken(s *slot, buf []byte) bool {
	// Sample from last logits position (-1).
	var token llama.Token
	switch {
	case s.grammarSampler != nil:
		token = s.grammarSampler.SampleWithGrammar(e.model.lctx, s.sampler, -1)

	default:
		token = llama.SamplerSample(s.sampler, e.model.lctx, -1)
	}

	// Process through full pipeline (logprobs, accept, stream, count).
	// This may call finishSlot on EOG/error/maxTokens.
	wasActive := s.active
	e.handleSampledToken(s, token, -1, buf)

	// Return false if slot was finished by handleSampledToken.
	return s.active == wasActive && s.active
}
