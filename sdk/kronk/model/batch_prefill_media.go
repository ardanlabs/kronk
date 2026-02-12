package model

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// addPrefillMediaChunk processes the next chunk of a media request.
// For text chunks, tokens are added to the shared batch.
// For image chunks, embeddings are encoded and decoded separately.
// Returns false if cancelled; true otherwise (even if still prefilling).
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

		nBatch := e.model.cfg.NBatch

		switch s.useMRoPE {
		case true:
			// M-RoPE: process all tokens via separate decode (doesn't use shared batch).
			for start := s.chunkTokIdx; start < len(tokens); start += nBatch {
				end := min(start+nBatch, len(tokens))
				batchTokens := tokens[start:end]

				if err := e.decodeTextMRoPE(s, batchTokens); err != nil {
					e.finishSlot(s, fmt.Errorf("decode text chunk (M-RoPE) failed: %w", err))
					return false
				}
			}
			s.chunkTokIdx = 0
			s.chunkIdx++

		case false:
			// Non-M-RoPE: add tokens to shared batch with capacity check.
			remaining := len(tokens) - s.chunkTokIdx
			availableInBatch := nBatch - int(e.batch.NTokens)

			if availableInBatch <= 0 {
				s.iBatch = -1
				return true
			}

			chunkSize := min(remaining, availableInBatch)
			isLastChunk := s.chunkIdx == numChunks-1

			for i := range chunkSize {
				tokIdx := s.chunkTokIdx + i
				isLast := tokIdx == len(tokens)-1 && isLastChunk
				e.batch.Add(tokens[tokIdx], s.nPast, s.seqIDs, isLast)
				s.nPast++
			}
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
			s.inputChunks = 0
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
		if err := mtmd.EncodeChunk(s.job.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode image chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.job.mtmdCtx, embedSize)
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
			s.inputChunks = 0
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}
		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(prefillStart))

	case mtmd.InputChunkTypeAudio:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-audio",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", nTokens)

		// Step 1: Encode the audio chunk (runs through audio encoder).
		if err := mtmd.EncodeChunk(s.job.mtmdCtx, chunk); err != nil {
			e.finishSlot(s, fmt.Errorf("encode audio chunk failed: %w", err))
			return false
		}

		// Step 2: Retrieve the computed embeddings.
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embedSize := nEmbd * int32(nTokens)
		embd, err := mtmd.GetOutputEmbd(s.job.mtmdCtx, embedSize)
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
			s.inputChunks = 0
			if s.span.IsRecording() {
				s.span.SetAttributes(attribute.String("prefill-media", time.Since(prefillStart).String()))
			}

		case false:
			s.iBatch = -1
		}

		metrics.AddPrefillTime(e.model.modelInfo.ID, time.Since(prefillStart))
	}

	return true
}
