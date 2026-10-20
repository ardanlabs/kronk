package model

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// synchronizeDecode completes submitted target work. The caller holds decodeMu.
func (m *Model) synchronizeDecode() error {
	if err := llama.Synchronize(m.lctx); err != nil {
		return fmt.Errorf("synchronize target decode: %w", err)
	}
	m.pendingDecode = false
	return nil
}

// waitForPendingDecode is a boundary before target state is read, changed, or
// released. A mutex alone does not complete asynchronous GPU work.
func (m *Model) waitForPendingDecode() error {
	m.decodeMu.Lock()
	defer m.decodeMu.Unlock()
	if !m.pendingDecode {
		return nil
	}
	return m.synchronizeDecode()
}

// =============================================================================
// MTMD GENERATION BATCH DECODE HELPERS
// =============================================================================

// validateMediaBatchCapacity keeps non-causal chunks indivisible and prevents
// llama.cpp's native assertion when a chunk exceeds physical batch capacity.
func (m *Model) validateMediaBatchCapacity(nTokens int32, useNonCausal bool) error {
	if !useNonCausal || nTokens <= 0 {
		return nil
	}
	nBatch, nUBatch := m.cfg.EffectiveNBatch(), m.cfg.EffectiveNUBatch()
	if m.lctx != 0 {
		nBatch, nUBatch = int(llama.NBatch(m.lctx)), int(llama.NUBatch(m.lctx))
	}
	if int(nTokens) > min(nBatch, nUBatch) {
		return fmt.Errorf("non-causal media chunk requires %d tokens in one batch, but NBatch=%d and NUBatch=%d; increase ImageBatchTokenCapacity (image-batch-token-capacity) to at least %d", nTokens, nBatch, nUBatch, nTokens)
	}
	return nil
}

// decodeTextIsolated decodes text outside the shared generation tray.
// llama.cpp expands linear token positions for an M-RoPE context.
func (e *batchEngine) decodeTextIsolated(s *slot, tokens []llama.Token) error {
	if len(tokens) == 0 {
		return nil
	}

	batch := e.mropeBatch
	if err := stageMRoPEText(batch, tokens, s.nPast, s.seqIDs, extendedBatchOutputLogits); err != nil {
		return fmt.Errorf("stage M-RoPE text: %w", err)
	}

	e.model.decodeMu.Lock()
	ret, err := batch.process(llama.ProcessTypeDecode)
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
		err = e.syncMTPDecodedRows(s, batch, 0, s.nPast+llama.Pos(len(tokens)))
	}
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(len(tokens))
	return nil
}

// decodeEmbeddingsNormal decodes image embeddings with standard linear positioning.
// Used for non-M-RoPE models where positions are simply sequential integers.
func (e *batchEngine) decodeEmbeddingsNormal(s *slot, embd []float32, nEmbd, nTokens int32) error {
	if nTokens == 0 {
		return nil
	}
	positions := linearPositions(nTokens, s.nPast)
	return e.decodeMediaEmbeddings(s, embd, nEmbd, nTokens, positions, llama.Pos(nTokens))
}

// decodeEmbeddingsMRoPE decodes embeddings with M-RoPE positioning. Positions
// are laid out as 4 contiguous arrays:
//
//	[dim0: n_tokens] [dim1: n_tokens] [dim2: n_tokens] [dim3: n_tokens]
func (e *batchEngine) decodeEmbeddingsMRoPE(s *slot, embd []float32, nEmbd, nTokens int32, positions []llama.Pos, nPos llama.Pos) error {
	if len(positions) != int(nTokens*4) {
		return fmt.Errorf("mrope embedding positions: got %d, want %d", len(positions), nTokens*4)
	}
	return e.decodeMediaEmbeddings(s, embd, nEmbd, nTokens, positions, nPos)
}

// decodeMediaEmbeddings splits causal chunks to fit the native batch while
// keeping non-causal chunks intact and advancing the slot only after success.
func (e *batchEngine) decodeMediaEmbeddings(s *slot, embd []float32, nEmbd, nTokens int32, positions []llama.Pos, nPos llama.Pos) error {
	if nTokens == 0 {
		return nil
	}
	if err := e.model.validateMediaBatchCapacity(nTokens, s.useNonCausal); err != nil {
		return err
	}
	if nEmbd <= 0 || nTokens < 0 || len(embd) != int(nEmbd*nTokens) {
		return fmt.Errorf("embedding values: got %d, want %d rows of %d", len(embd), nTokens, nEmbd)
	}

	batch := e.mropeBatch
	nBatch := batch.capacity
	n := int(nTokens)
	positionCount := len(positions) / n

	e.model.decodeMu.Lock()
	defer e.model.decodeMu.Unlock()

	for start := 0; start < n; start += nBatch {
		end := min(start+nBatch, n)
		batchN := end - start
		// Positions are section-major: gather each plane, not a contiguous
		// slice spanning rows from different planes.
		subPositions := make([]llama.Pos, batchN*positionCount)
		for plane := range positionCount {
			copy(subPositions[plane*batchN:(plane+1)*batchN], positions[plane*n+start:plane*n+end])
		}
		output := extendedBatchOutputNone
		if end == n {
			output = extendedBatchOutputLogits
		}
		if err := stageEmbeddingRows(batch, embd[start*int(nEmbd):end*int(nEmbd)], nEmbd, int32(batchN), subPositions, s.seqIDs, output); err != nil {
			return fmt.Errorf("stage media embeddings at row %d: %w", start, err)
		}

		wasCausal := false
		if s.useNonCausal {
			wasCausal = llama.GetCausalAttn(e.model.lctx)
			llama.SetCausalAttn(e.model.lctx, false)
		}
		ret, err := batch.process(llama.ProcessTypeDecode)
		if s.useNonCausal {
			llama.SetCausalAttn(e.model.lctx, wasCausal)
		}
		if err != nil || ret != 0 {
			return decodeError(ret, err)
		}
		llama.Synchronize(e.model.lctx)
		nextPosition := s.nPast + nPos
		if positionCount == 1 {
			nextPosition = s.nPast + llama.Pos(end)
		}
		// M-RoPE's logical end is authoritative, as in the IMC decoder.
		if err := e.syncMTPDecodedRows(s, batch, 0, nextPosition); err != nil {
			return err
		}
	}

	s.nPast += nPos
	return nil
}

func stageMRoPEText(batch *extendedBatch, tokens []llama.Token, start llama.Pos, sequenceIDs []llama.SeqId, finalOutput extendedBatchOutput) error {
	batch.clear()

	for i, token := range tokens {
		position := start + llama.Pos(i)
		output := extendedBatchOutputNone
		if i == len(tokens)-1 {
			output = finalOutput
		}

		if _, err := batch.addToken(token, position, sequenceIDs, output); err != nil {
			return fmt.Errorf("add token at position %d: %w", position, err)
		}
	}

	return nil
}

func stageEmbeddingRows(batch *extendedBatch, embd []float32, nEmbd, nTokens int32, positions []llama.Pos, sequenceIDs []llama.SeqId, finalOutput extendedBatchOutput) error {
	batch.clear()
	return appendEmbeddingRows(batch, embd, nEmbd, nTokens, positions, sequenceIDs, finalOutput)
}

func appendEmbeddingRows(batch *extendedBatch, embd []float32, nEmbd, nTokens int32, positions []llama.Pos, sequenceIDs []llama.SeqId, finalOutput extendedBatchOutput) error {
	if nEmbd <= 0 || nTokens <= 0 || len(embd) != int(nEmbd*nTokens) {
		return fmt.Errorf("embedding values: got %d, want %d rows of %d", len(embd), nTokens, nEmbd)
	}
	if len(positions)%int(nTokens) != 0 {
		return fmt.Errorf("embedding positions: got %d for %d rows", len(positions), nTokens)
	}

	positionCount := len(positions) / int(nTokens)
	if positionCount < 1 || positionCount > 4 {
		return fmt.Errorf("embedding positions: got %d planes, want 1 to 4", positionCount)
	}

	for i := range nTokens {
		start := int(i * nEmbd)
		row := embd[start : start+int(nEmbd)]

		var rowPositions [4]llama.Pos
		for plane := range positionCount {
			rowPositions[plane] = positions[int(i)+plane*int(nTokens)]
		}

		output := extendedBatchOutputNone
		if i == nTokens-1 {
			output = finalOutput
		}
		if _, err := batch.addEmbedding(row, int(nEmbd), rowPositions[:positionCount], sequenceIDs, output); err != nil {
			return fmt.Errorf("add embedding row %d: %w", i, err)
		}
	}

	return nil
}

func fillMRoPETextPositions(positions []llama.Pos, n int32, start llama.Pos) {
	for i := range n {
		pos := start + llama.Pos(i)
		positions[i] = pos
		positions[i+n] = pos
		positions[i+n*2] = pos
		positions[i+n*3] = pos
	}
}

func linearMRoPEPositions(n int32, start llama.Pos) []llama.Pos {
	positions := make([]llama.Pos, n*4)
	fillMRoPETextPositions(positions, n, start)
	return positions
}

func linearPositions(n int32, start llama.Pos) []llama.Pos {
	positions := make([]llama.Pos, n)
	for i := range n {
		positions[i] = start + llama.Pos(i)
	}
	return positions
}
