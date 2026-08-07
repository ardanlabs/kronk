package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"github.com/hybridgroup/yzma/pkg/llama"
	"go.opentelemetry.io/otel/attribute"
)

// processSlotToken samples and processes a generated token for a slot.
func (e *batchEngine) processSlotToken(s *slot, buf []byte) {
	// Sample the next token. If grammar is active, use grammar-aware sampling
	// but only when the parser is in the completion phase. During the
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
// logprobs extraction, grammar acceptance, EOG check, state machine, streaming,
// and token counting. SamplerSample already accepts the selected token on the
// main sampler. Used by both processSlotToken and sampleFirstToken.
func (e *batchEngine) handleSampledToken(s *slot, token llama.Token, iBatch int32, buf []byte) {
	e.handleToken(s, token, iBatch, buf, true, false, nil)
}

func (e *batchEngine) handleToken(s *slot, token llama.Token, iBatch int32, buf []byte, samplerAccepted bool, logprobReady bool, precomputedLogprob *ContentLogprob) {
	if isSuppressedToken(e.model.suppressTokens, token) {
		e.finishSlot(s, fmt.Errorf("sampler selected model-suppressed token %d", token))
		return
	}

	// Extract logprobs before any manually required acceptance. Reset
	// currentLogprob each token; it's used for streaming.
	s.currentLogprob = nil
	if s.job.params.Logprobs {
		logprob := precomputedLogprob
		if !logprobReady {
			var err error
			logprob, err = extractLogprobs(e.model.lctx, e.model.vocab, e.model.suppressTokens, token, iBatch, s.job.params.TopLogprobs, buf)
			if err != nil {
				e.model.log(s.job.ctx, "batch-engine", "status", "logprobs-error", "slot", s.id, "error", err.Error())
			}
		}
		if logprob != nil {
			s.currentLogprob = logprob
			if s.stopGate == nil {
				s.logprobsData = append(s.logprobsData, *logprob)
			}
		}
	}

	// Accept the grammar sampler separately to avoid the crash that occurs when
	// grammar is in the main chain. Skip
	// grammar acceptance during reasoning — reasoning tokens are not
	// grammar-constrained and must not advance the grammar state machine.
	if s.grammarSampler != nil && s.reasonFlag == 0 {
		s.grammarSampler.Accept(token)
	}

	if !samplerAccepted {
		llama.SamplerAccept(s.sampler, token)
	}

	// Check for end of generation.
	if llama.VocabIsEOG(e.model.vocab, token) {
		s.stopSource = "vocab-eog"
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
	// the parser and streaming.
	if len(content) == 0 {
		if s.stopGate != nil && s.currentLogprob != nil {
			s.stopUTF8Logprobs = append(s.stopUTF8Logprobs, s.currentLogprob)
		}

		switch {
		case s.reasonFlag > 0:
			s.reasonTokens++
		default:
			s.completionTokens++
		}
		if s.processingSpecToken {
			s.specCoveredTotal++
		}

		outputTokens := s.reasonTokens + s.completionTokens

		if outputTokens >= s.job.params.MaxTokens {
			s.finishReason = FinishReasonLength
			s.stopSource = "max-tokens"
			e.finishSlot(s, nil)
			return
		}

		s.iBatch = -1
		return
	}

	// Preserve the original immediate path when request stops are not active.
	// In particular, classification precedes accounting and max-token checks.
	if s.stopGate == nil {
		outcome := e.processDecodedPiece(s, stopPiece{content: content, logprob: s.currentLogprob}, len(s.logprobsData)-1, false)
		if outcome.parserEOG {
			s.stopSource = "parser-eog"
			e.finishSlot(s, nil)
			return
		}
		if outcome.err != nil {
			e.finishSlot(s, outcome.err)
			return
		}

		outputTokens := s.reasonTokens + s.completionTokens
		if outputTokens >= s.job.params.MaxTokens {
			s.finishReason = FinishReasonLength
			s.stopSource = "max-tokens"
			e.finishSlot(s, nil)
			return
		}

		s.iBatch = -1
		return
	}

	piece := stopPiece{
		content:           content,
		logprob:           s.currentLogprob,
		utf8Logprobs:      s.stopUTF8Logprobs,
		provisionalReason: s.reasonFlag > 0,
		speculative:       s.processingSpecToken,
	}
	s.stopUTF8Logprobs = nil
	accountStopPiece(s, piece)
	pieces, matched := s.stopGate.feed(piece)

	for _, piece := range pieces {
		logprobIndex := appendStopPieceLogprobs(s, piece)
		outcome := e.processDecodedPiece(s, piece, logprobIndex, true)
		if outcome.parserEOG {
			unaccountStopPiece(s, piece)
			s.stopSource = "parser-eog"
			e.finishSlot(s, nil)
			return
		}
		if outcome.err != nil {
			e.finishSlot(s, outcome.err)
			return
		}
	}

	if matched {
		for _, piece := range s.stopGate.takeDiscarded() {
			reconcileStopPiece(s, piece)
		}
		s.finishReason = FinishReasonStop
		s.stopSource = "request-stop"
		e.finishSlot(s, nil)
		return
	}

	outputTokens := s.reasonTokens + s.completionTokens
	if outputTokens >= s.job.params.MaxTokens {
		s.finishReason = FinishReasonLength
		s.stopSource = "max-tokens"
		e.finishSlot(s, nil)
		return
	}

	s.iBatch = -1
}

type decodedPieceOutcome struct {
	parserEOG bool
	err       error
}

// processDecodedPiece passes one released stop-gate piece through the parser.
// It reports terminal conditions to its caller and never finalizes the slot.
func (e *batchEngine) processDecodedPiece(s *slot, piece stopPiece, logprobIndex int, reconcile bool) decodedPieceOutcome {
	result, eog := s.stateMachine.Classify(piece.content)
	if s.suppressTools && result.Channel == ChannelTool {
		result.Channel = ChannelAnswer
	}

	if eog {
		return decodedPieceOutcome{parserEOG: true}
	}

	previousChannel := slotChannel(s)

	updateSlotChannel(s, result.Channel)
	if reconcile {
		reconcileStopPiece(s, piece)
	} else {
		accountCurrentChannel(s, s.processingSpecToken)
	}

	if result.Channel != ChannelNone && previousChannel != result.Channel {
		args := []any{
			"status", "channel-transition",
			"id", s.job.id,
			"from", channelName(previousChannel),
			"to", channelName(result.Channel),
		}
		if result.Channel == ChannelTool {
			args = append(args, "tool_buffering", "started")
		}
		e.model.log(s.job.ctx, "chat-completion", args...)
	}

	if streamer, ok := s.stateMachine.(ToolCallDeltaStreamer); ok && !s.suppressTools {
		deltas := streamer.ToolCallDeltas()
		if s.job.params.Stream {
			for _, delta := range deltas {
				if err := e.model.sendToolCallDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, delta); err != nil {
					return decodedPieceOutcome{err: err}
				}
			}
		}
	}

	// Non-streamable tokens (ChannelNone) have been counted above but have
	// no content to stream or further process.
	if result.Channel == ChannelNone {
		return decodedPieceOutcome{}
	}

	outputTokens := s.reasonTokens + s.completionTokens

	if err := e.retainAndStreamResult(s, result, outputTokens, piece.logprob, logprobIndex); err != nil {
		return decodedPieceOutcome{err: err}
	}

	return decodedPieceOutcome{}
}

func accountCurrentChannel(s *slot, speculative bool) {
	if s.reasonFlag > 0 {
		s.reasonTokens++
	} else {
		s.completionTokens++
	}
	if speculative {
		s.specCoveredTotal++
	}
}

func accountStopPiece(s *slot, piece stopPiece) {
	if piece.provisionalReason {
		s.reasonTokens++
	} else {
		s.completionTokens++
	}
	if piece.speculative {
		s.specCoveredTotal++
	}
}

func unaccountStopPiece(s *slot, piece stopPiece) {
	if piece.provisionalReason {
		s.reasonTokens--
	} else {
		s.completionTokens--
	}
	if piece.speculative {
		s.specCoveredTotal--
	}
}

func appendStopPieceLogprobs(s *slot, piece stopPiece) int {
	for _, logprob := range piece.utf8Logprobs {
		s.logprobsData = append(s.logprobsData, *logprob)
	}
	if piece.logprob == nil {
		return -1
	}

	s.logprobsData = append(s.logprobsData, *piece.logprob)
	return len(s.logprobsData) - 1
}

func reconcileStopPiece(s *slot, piece stopPiece) {
	actualReason := s.reasonFlag > 0
	if actualReason == piece.provisionalReason {
		return
	}
	if actualReason {
		s.reasonTokens++
		s.completionTokens--
		return
	}
	s.reasonTokens--
	s.completionTokens++
}

// retainAndStreamResult applies response cleanup once, before both the final
// accumulators and streaming deltas consume the content.
func (e *batchEngine) retainAndStreamResult(s *slot, result Result, outputTokens int, logprob *ContentLogprob, logprobIndex int) error {
	if result.Channel != ChannelTool && e.model.isUnnecessaryCRLF(s.reasonFlag, s.completionFlag, result.Content) {
		if logprob != nil && logprobIndex >= 0 && logprobIndex < len(s.logprobsData) {
			s.logprobsData = append(s.logprobsData[:logprobIndex], s.logprobsData[logprobIndex+1:]...)
			s.currentLogprob = nil
		}
		return nil
	}

	switch result.Channel {
	case ChannelReasoning:
		s.finalReasoning.WriteString(result.Content)

	case ChannelTool:
		s.finalTooling.WriteString(result.Content)
		return nil

	default:
		s.finalContent.WriteString(result.Content)
	}

	// Per OpenAI spec, usage is only sent in the final response, not deltas.
	return e.model.sendDeltaResponse(s.job.ctx, s.job.ch, s.job.id, s.job.object, 0, result.Content, result.Channel, s.reasonTokens, outputTokens, logprob)
}

func updateSlotChannel(s *slot, channel Channel) {
	switch channel {
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
}

// handleSpeculativeToken processes an accepted draft or bonus token and marks
// emitted output for speculative coverage accounting.
func (e *batchEngine) handleSpeculativeToken(s *slot, token llama.Token, iBatch int32, buf []byte, samplerAccepted bool, logprobReady bool, logprob *ContentLogprob) {
	s.processingSpecToken = true
	e.handleToken(s, token, iBatch, buf, samplerAccepted, logprobReady, logprob)
	if s.active {
		s.processingSpecToken = false
	}
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
