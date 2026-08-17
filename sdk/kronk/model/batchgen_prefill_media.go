package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

func (e *batchEngine) nextMediaSlot() (*slot, int) {
	for offset := range e.slots {
		idx := (e.mediaNext + offset) % len(e.slots)
		s := e.slots[idx]
		if s.active && s.inputChunks != 0 && !s.mediaPrefillDone {
			return s, idx
		}
	}

	return nil, -1
}

func (e *batchEngine) mediaChunkUsesSharedBatch(s *slot) bool {
	if s.inputChunks == 0 || s.mediaPrefillDone || s.chunkIdx >= int(mtmd.InputChunksSize(s.inputChunks)) {
		return false
	}

	chunk := mtmd.InputChunksGet(s.inputChunks, uint64(s.chunkIdx))
	return mtmd.InputChunkGetType(chunk) == mtmd.InputChunkTypeText && !s.useMRoPE
}

func (e *batchEngine) processMediaSlot(s *slot, idx int, buf []byte) {
	if s.job.ctx.Err() != nil {
		e.finishSlot(s, s.job.ctx.Err())
		e.mediaNext = (idx + 1) % len(e.slots)
		return
	}

	// Publish the media-prefill phase before potentially slow projector or
	// embedding decode work so diagnostics clients can observe the overlap.
	e.publishDiagnostics(true)
	if !e.addPrefillMediaChunk(s, buf) && s.job != nil {
		e.finishSlot(s, e.slotCancelError(s))
	}
	e.mediaNext = (idx + 1) % len(e.slots)
}

// addPrefillMediaChunk processes the next chunk of a generation media request.
// For text chunks, tokens are added to the shared batch.
// For image chunks, embeddings are encoded and decoded separately.
// Returns false if cancelled or an internal error occurs; true otherwise (even
// if still prefilling). Internal errors finish the slot before returning.
func (e *batchEngine) addPrefillMediaChunk(s *slot, buf []byte) bool {
	numChunks := int(mtmd.InputChunksSize(s.inputChunks))

	// Check if all chunks have been processed.
	if s.chunkIdx >= numChunks {
		return true
	}

	// Check for cancellation.
	select {
	case <-e.shutdownCh:
		return false

	case <-s.job.ctx.Done():
		return false

	default:
	}

	prefillStart := time.Now()
	chunk := mtmd.InputChunksGet(s.inputChunks, uint64(s.chunkIdx))
	chunkType := mtmd.InputChunkGetType(chunk)
	nTokens := mtmd.InputChunkGetNTokens(chunk)

	switch chunkType {
	case mtmd.InputChunkTypeText:
		tokens := mtmd.InputChunkGetTokensText(chunk)
		if len(tokens) == 0 {
			s.chunkIdx++
			s.chunkTokIdx = 0
			return true
		}

		nBatch := e.model.cfg.EffectiveNBatch()
		chunkLimit := e.model.cfg.PrefillBatchSize()

		switch s.useMRoPE {
		case true:
			// M-RoPE text uses a separate decode, capped to one resumable
			// prefill unit so the scheduler can return to generation.
			remaining := len(tokens) - s.chunkTokIdx
			chunkSize := mediaTextContributionSize(remaining, nBatch, chunkLimit)
			end := s.chunkTokIdx + chunkSize
			if err := e.decodeTextMRoPE(s, tokens[s.chunkTokIdx:end]); err != nil {
				e.finishSlot(s, fmt.Errorf("decode text chunk (M-RoPE) failed: %w", err))
				return false
			}
			s.chunkTokIdx = end
			if s.chunkTokIdx >= len(tokens) {
				s.chunkTokIdx = 0
				s.chunkIdx++
			}

		case false:
			// Non-M-RoPE: add tokens to shared batch with capacity check.
			remaining := len(tokens) - s.chunkTokIdx
			availableInBatch := nBatch - int(e.batch.NTokens)

			if availableInBatch <= 0 {
				s.iBatch = -1
				return true
			}

			chunkSize := mediaTextContributionSize(remaining, availableInBatch, chunkLimit)
			isLastChunk := s.chunkIdx == numChunks-1

			batchStart := e.batch.NTokens
			for i := range chunkSize {
				tokIdx := s.chunkTokIdx + i
				isLast := tokIdx == len(tokens)-1 && isLastChunk
				if err := e.batch.Add(tokens[tokIdx], s.nPast+llama.Pos(i), s.seqIDs, isLast); err != nil {
					e.batch.NTokens = batchStart
					e.finishSlot(s, fmt.Errorf("add media prefill token %d: %w", tokIdx, err))
					return false
				}
			}
			s.nPast += llama.Pos(chunkSize)
			s.chunkTokIdx += chunkSize

			// Check if text chunk is complete.
			switch s.chunkTokIdx >= len(tokens) {
			case true:
				s.chunkTokIdx = 0
				s.chunkIdx++

			case false:
				s.iBatch = -1
				return true
			}
		}

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			switch s.useMRoPE {
			case true:
				// M-RoPE text uses separate decode, so we must sample the first
				// token immediately since nothing was added to the shared batch.
				if !e.sampleFirstToken(s, buf) {
					return false
				}
			case false:
				// Non-M-RoPE text was added to shared batch, sample after decode.
				s.iBatch = e.batch.NTokens - 1
			}
			s.mediaPrefillDone = true
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}
		case false:
			s.iBatch = -1
		}

	case mtmd.InputChunkTypeImage:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-image",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", nTokens)

		// Step 1: Encode the image chunk (runs through vision encoder).
		if err := mtmd.EncodeChunk(s.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode image chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.mtmdCtx, embedSize)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("get image embeddings failed: %w", err))
			return false
		}

		// Step 3: Decode embeddings into the LLM's KV cache.
		// This uses a separate decode call since embeddings can't batch with tokens.
		switch s.useMRoPE {
		case true:
			imageTokens := mtmd.InputChunkGetTokensImage(chunk)
			nx := int32(mtmd.ImageTokensGetNX(imageTokens))
			ny := int32(mtmd.ImageTokensGetNY(imageTokens))

			e.model.log(s.job.ctx, "prefill-media", "status", "decoding-image-mrope",
				"slot", s.id, "nx", nx, "ny", ny)

			if err := e.decodeEmbeddingsMRoPE(s, embd, nEmbd, int32(nTokens), nx, ny); err != nil {
				e.finishSlot(s, fmt.Errorf("decode image embeddings (M-RoPE) failed: %w", err))
				return false
			}

		case false:
			if err := e.decodeEmbeddingsNormal(s, embd, nEmbd, int32(nTokens)); err != nil {
				e.finishSlot(s, fmt.Errorf("decode image embeddings failed: %w", err))
				return false
			}
		}

		s.chunkIdx++

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			// Image chunks use separate decode, so we must sample the first
			// token immediately since nothing was added to the shared batch.
			if !e.sampleFirstToken(s, buf) {
				return false
			}
			s.mediaPrefillDone = true
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}
		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, "media", time.Since(prefillStart))

	case mtmd.InputChunkTypeAudio:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-audio",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", nTokens)

		// Step 1: Encode the audio chunk (runs through audio encoder).
		if err := mtmd.EncodeChunk(s.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode audio chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.mtmdCtx, embedSize)
		if err != nil {
			e.finishSlot(s, fmt.Errorf("get audio embeddings failed: %w", err))
			return false
		}

		// Step 3: Decode embeddings into the LLM's KV cache.
		// Audio uses standard linear positioning (not M-RoPE).
		if err := e.decodeEmbeddingsNormal(s, embd, nEmbd, int32(nTokens)); err != nil {
			e.finishSlot(s, fmt.Errorf("decode audio embeddings failed: %w", err))
			return false
		}

		s.chunkIdx++

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
			// Audio uses separate decode, so sample first token immediately.
			if !e.sampleFirstToken(s, buf) {
				return false
			}
			s.mediaPrefillDone = true
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}

		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, "media", time.Since(prefillStart))
	}

	return true
}

func mediaTextContributionSize(remaining, availableInBatch, chunkLimit int) int {
	return max(min(remaining, availableInBatch, chunkLimit), 0)
}
