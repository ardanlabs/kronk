package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/observ/metrics"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"go.opentelemetry.io/otel/attribute"
)

// imageTokensDecoderPositions returns the four section-major decoder position
// planes for image tokens. The returned planes follow llama_batch ordering:
// temporal, y, x, then z.
func imageTokensDecoderPositions(imageTokens mtmd.ImageTokens, start llama.Pos, nTokens int32) ([]llama.Pos, error) {
	if imageTokens == 0 {
		return nil, fmt.Errorf("image tokens are nil")
	}
	if nTokens <= 0 {
		return nil, fmt.Errorf("invalid image token count %d", nTokens)
	}

	positions := make([]llama.Pos, nTokens*4)
	for i := range nTokens {
		pos := mtmd.ImageTokensGetDecoderPos(imageTokens, start, uint64(i))

		positions[i] = llama.Pos(pos.T)
		positions[i+nTokens] = llama.Pos(pos.Y)
		positions[i+nTokens*2] = llama.Pos(pos.X)
		positions[i+nTokens*3] = llama.Pos(pos.Z)
	}

	return positions, nil
}

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

func (e *batchEngine) planMediaSlot(s *slot) batchContributionPlan {
	if s.inputChunks == 0 || s.mediaPrefillDone || s.chunkIdx >= int(mtmd.InputChunksSize(s.inputChunks)) {
		return batchContributionPlan{}
	}

	chunk := mtmd.InputChunksGet(s.inputChunks, uint64(s.chunkIdx))
	chunkType := mtmd.InputChunkGetType(chunk)
	nRows := int(mtmd.InputChunkGetNTokens(chunk))
	requirements := batchContributionRequirements{
		positions: batchPositionLinear,
		attention: batchAttentionCausal,
		rows:      nRows,
	}
	if s.useMRoPE {
		requirements.positions = batchPositionMRoPE
	}

	switch chunkType {
	case mtmd.InputChunkTypeText:
		requirements.input = batchInputTokens
		requirements.rows = len(mtmd.InputChunkGetTokensText(chunk)) - s.chunkTokIdx

	case mtmd.InputChunkTypeImage, mtmd.InputChunkTypeAudio:
		requirements.input = batchInputEmbeddings
		requirements.mustFit = true
		if s.useNonCausal {
			requirements.attention = batchAttentionNonCausal
		}

	default:
		return batchContributionPlan{mode: batchExecutionIsolated, rows: nRows}
	}

	if requirements.rows <= 0 {
		return batchContributionPlan{mode: batchExecutionIsolated}
	}

	return planBatchContribution(
		e.model.modelInfo.profile.Batch.InputMixing,
		requirements,
		e.model.cfg.EffectiveNBatch()-e.batch.len(),
		e.model.cfg.PrefillBatchSize(),
	)
}

type stagedMediaContribution struct {
	slot            *slot
	nextChunkIdx    int
	nextChunkTokIdx int
	nextPast        llama.Pos
	outputIndex     int32
	complete        bool
	media           bool
	modelID         string
	started         time.Time
}

func (c stagedMediaContribution) commit() {
	c.slot.chunkIdx = c.nextChunkIdx
	c.slot.chunkTokIdx = c.nextChunkTokIdx
	c.slot.nPast = c.nextPast
	c.slot.iBatch = c.outputIndex
	c.slot.mediaPrefillDone = c.complete

	duration := time.Since(c.started)
	if c.complete && c.slot.span.IsRecording() {
		c.slot.span.SetAttributes(attribute.String("prefill-media", duration.String()))
	}
	if c.media {
		metrics.AddPrefillTime(c.modelID, "media", duration)
	}
}

func (e *batchEngine) stageMediaSlot(s *slot, plan batchContributionPlan) (*stagedMediaContribution, error) {
	if plan.mode != batchExecutionShared || plan.rows <= 0 {
		return nil, fmt.Errorf("stage media slot: invalid shared plan %+v", plan)
	}
	if err := s.job.ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case <-e.shutdownCh:
		return nil, fmt.Errorf("stage media slot: engine shutting down")
	default:
	}

	started := time.Now()
	numChunks := int(mtmd.InputChunksSize(s.inputChunks))
	chunk := mtmd.InputChunksGet(s.inputChunks, uint64(s.chunkIdx))
	chunkType := mtmd.InputChunkGetType(chunk)
	batchStart := e.batch.len()
	contribution := stagedMediaContribution{
		slot:            s,
		nextChunkIdx:    s.chunkIdx,
		nextChunkTokIdx: s.chunkTokIdx,
		nextPast:        s.nPast,
		outputIndex:     -1,
		modelID:         e.model.modelInfo.ID,
		started:         started,
	}

	switch chunkType {
	case mtmd.InputChunkTypeText:
		tokens := mtmd.InputChunkGetTokensText(chunk)
		end := s.chunkTokIdx + plan.rows
		if end > len(tokens) {
			return nil, fmt.Errorf("stage media text: end %d exceeds %d tokens", end, len(tokens))
		}
		for i, token := range tokens[s.chunkTokIdx:end] {
			lastChunk := s.chunkIdx == numChunks-1
			lastRow := end == len(tokens) && i == plan.rows-1
			output := extendedBatchOutputNone
			if lastChunk && lastRow {
				output = extendedBatchOutputLogits
			}
			if _, err := e.batch.addToken(token, s.nPast+llama.Pos(i), s.seqIDs, output); err != nil {
				return nil, fmt.Errorf("stage media text token %d: %w", s.chunkTokIdx+i, errors.Join(err, e.batch.truncate(batchStart)))
			}
		}

		contribution.nextPast += llama.Pos(plan.rows)
		contribution.nextChunkTokIdx = end
		if end == len(tokens) {
			contribution.nextChunkIdx++
			contribution.nextChunkTokIdx = 0
		}

	case mtmd.InputChunkTypeImage, mtmd.InputChunkTypeAudio:
		e.model.log(s.job.ctx, "prefill-media", "status", "encoding-shared",
			"slot", s.id, "chunk", s.chunkIdx, "tokens", plan.rows)
		if err := mtmd.EncodeChunk(s.mtmdCtx, chunk); err != nil {
			return nil, fmt.Errorf("encode shared media chunk: %w", err)
		}

		nTokens := int32(plan.rows)
		nEmbd := llama.ModelNEmbdInp(e.model.model)
		embd, err := mtmd.GetOutputEmbd(s.mtmdCtx, nEmbd*nTokens)
		if err != nil {
			return nil, fmt.Errorf("get shared media embeddings: %w", err)
		}

		var (
			positions []llama.Pos
			nPos      llama.Pos
		)
		switch {
		case s.useMRoPE && chunkType == mtmd.InputChunkTypeImage:
			positions, err = imageTokensDecoderPositions(mtmd.InputChunkGetTokensImage(chunk), s.nPast, nTokens)
			nPos = llama.Pos(mtmd.InputChunkGetNPos(chunk))
		case s.useMRoPE:
			positions = linearMRoPEPositions(nTokens, s.nPast)
			nPos = llama.Pos(mtmd.InputChunkGetNPos(chunk))
		default:
			positions = linearPositions(nTokens, s.nPast)
			nPos = llama.Pos(nTokens)
		}
		if err != nil {
			return nil, fmt.Errorf("get shared media positions: %w", err)
		}

		output := extendedBatchOutputNone
		if s.chunkIdx == numChunks-1 {
			output = extendedBatchOutputLogits
		}
		if err := appendEmbeddingRows(e.batch, embd, nEmbd, nTokens, positions, s.seqIDs, output); err != nil {
			return nil, fmt.Errorf("stage shared media embeddings: %w", errors.Join(err, e.batch.truncate(batchStart)))
		}

		contribution.nextPast += nPos
		contribution.nextChunkIdx++
		contribution.nextChunkTokIdx = 0
		contribution.media = true

	default:
		return nil, fmt.Errorf("stage media slot: unsupported chunk type %d", chunkType)
	}

	contribution.complete = contribution.nextChunkIdx >= numChunks
	if contribution.complete {
		contribution.outputIndex = int32(e.batch.len() - 1)
	}

	return &contribution, nil
}

func (e *batchEngine) processIsolatedMediaSlot(s *slot, idx int, buf []byte) {
	if s.job.ctx.Err() != nil {
		e.finishSlot(s, s.job.ctx.Err())
		e.mediaNext = (idx + 1) % len(e.slots)
		return
	}

	// Publish the media-prefill phase before potentially slow projector or
	// embedding decode work so diagnostics clients can observe the overlap.
	e.publishDiagnostics(true)
	if !e.processIsolatedMediaChunk(s, buf) && s.job != nil {
		e.finishSlot(s, e.slotCancelError(s))
	}
	e.mediaNext = (idx + 1) % len(e.slots)
}

// processIsolatedMediaChunk processes one media contribution outside the
// shared tray.
// Returns false if cancelled or an internal error occurs; true otherwise (even
// if still prefilling). Internal errors finish the slot before returning.
func (e *batchEngine) processIsolatedMediaChunk(s *slot, buf []byte) bool {
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

		remaining := len(tokens) - s.chunkTokIdx
		chunkSize := mediaTextContributionSize(remaining, e.model.cfg.EffectiveNBatch(), e.model.cfg.PrefillBatchSize())
		end := s.chunkTokIdx + chunkSize
		if err := e.decodeTextIsolated(s, tokens[s.chunkTokIdx:end]); err != nil {
			e.finishSlot(s, fmt.Errorf("decode isolated text chunk: %w", err))
			return false
		}
		s.chunkTokIdx = end
		if s.chunkTokIdx >= len(tokens) {
			s.chunkTokIdx = 0
			s.chunkIdx++
		}

		// Check if this was the last chunk.
		switch s.chunkIdx >= numChunks {
		case true:
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

		// Step 3: Decode embeddings into the LLM's KV cache using the isolated
		// plan selected for non-causal, unsupported, or capacity-constrained work.
		switch s.useMRoPE {
		case true:
			imageTokens := mtmd.InputChunkGetTokensImage(chunk)
			positions, err := imageTokensDecoderPositions(imageTokens, s.nPast, int32(nTokens))
			if err != nil {
				e.finishSlot(s, fmt.Errorf("get image decoder positions: %w", err))
				return false
			}
			nPos := llama.Pos(mtmd.InputChunkGetNPos(chunk))

			e.model.log(s.job.ctx, "prefill-media", "status", "decoding-image-mrope",
				"slot", s.id, "tokens", nTokens, "positions", nPos)

			if err := e.decodeEmbeddingsMRoPE(s, embd, nEmbd, int32(nTokens), positions, nPos); err != nil {
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
		switch s.useMRoPE {
		case true:
			positions := linearMRoPEPositions(int32(nTokens), s.nPast)
			nPos := llama.Pos(mtmd.InputChunkGetNPos(chunk))

			if err := e.decodeEmbeddingsMRoPE(s, embd, nEmbd, int32(nTokens), positions, nPos); err != nil {
				e.finishSlot(s, fmt.Errorf("decode audio embeddings (M-RoPE) failed: %w", err))
				return false
			}

		case false:
			if err := e.decodeEmbeddingsNormal(s, embd, nEmbd, int32(nTokens)); err != nil {
				e.finishSlot(s, fmt.Errorf("decode audio embeddings failed: %w", err))
				return false
			}
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
