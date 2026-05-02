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
	// Sample the next token. If grammar is active, use grammar-aware sampling
	// but only when the processor is in the completion phase. During the
	// reasoning phase (<think>...</think>), grammar constraints would corrupt
	// the thinking tokens and prevent the model from closing the think block.
	var token llama.Token
	switch {
	case s.grammarSampler != nil && s.reasonFlag == 0:
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
	// to avoid the crash that occurs when grammar is in the chain. Skip
	// grammar acceptance during reasoning — reasoning tokens are not
	// grammar-constrained and must not advance the grammar state machine.
	if s.grammarSampler != nil && s.reasonFlag == 0 {
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
		metrics.AddPrefillTTFT(e.model.modelInfo.ID, ttft)

		// End-to-end TTFT: from the moment the request entered the SDK
		// (ChatStreaming/Chat) to the first sampled token. Includes
		// queue wait, tokenization, cache work, and prefill.
		if !s.job.requestStart.IsZero() {
			metrics.AddRequestTTFT(e.model.modelInfo.ID, time.Since(s.job.requestStart))
		}

		e.model.log(s.job.ctx, "batch-engine", "status", "prefill-done",
			"slot", s.id, "seq", s.seqID, "id", s.job.id,
			"prompt_tokens", s.nPrompt, "ttft", ttft.String())

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

	// Process through the processor-plugin state machine.
	result, eog := s.stateMachine.Process(content)

	if eog {
		e.finishSlot(s, nil)
		return
	}

	// Update flags based on the classified channel.
	switch result.Channel {
	case ChannelReasoning:
		s.reasonFlag++
		s.completionFlag = 0
		s.toolFlag = 0

	case ChannelAnswer:
		s.completionFlag++
		s.reasonFlag = 0
		s.toolFlag = 0

	case ChannelTool:
		s.toolFlag++
		s.reasonFlag = 0
		s.completionFlag = 0
	}

	// Count every token the model generates. EOG tokens are handled before
	// the state machine (line 55), so everything here is a real output token.
	// Tokens classified as ChannelNone (control tags, buffered tool-call
	// content) are still generated by the model and must be counted for
	// accurate usage reporting.
	switch {
	case s.reasonFlag > 0:
		s.reasonTokens++
	default:
		s.completionTokens++
	}

	// Non-streamable tokens (ChannelNone) have been counted above but have
	// no content to stream or further process.
	if result.Channel == ChannelNone {
		s.iBatch = -1
		return
	}

	outputTokens := s.reasonTokens + s.completionTokens

	if outputTokens >= s.job.params.MaxTokens {
		e.finishSlot(s, nil)
		return
	}

	// Store content for final response.
	switch {
	case s.reasonFlag > 0:
		s.finalReasoning.WriteString(result.Content)

	case s.toolFlag > 0:
		s.finalTooling.WriteString(result.Content)

	default:
		s.finalContent.WriteString(result.Content)
	}

	// Stream response if not tooling.
	if s.toolFlag == 0 {
		// Skip unnecessary CRLF at mode transitions.
		if e.model.isUnnecessaryCRLF(s.reasonFlag, s.completionFlag, result.Content) {
			s.iBatch = -1
			return
		}

		// Per OpenAI spec, usage is only sent in the final response, not deltas.
		err := e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, "", result.Content, s.reasonFlag, outputTokens, s.currentLogprob)
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
	// Sample from last logits position (-1). Skip grammar during reasoning.
	var token llama.Token
	switch {
	case s.grammarSampler != nil && s.reasonFlag == 0:
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
