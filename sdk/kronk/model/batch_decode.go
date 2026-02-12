package model

import (
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// MTMD BATCH DECODE HELPERS
// =============================================================================

// decodeTextMRoPE decodes text tokens for M-RoPE models.
// M-RoPE uses 4D positions: [dim0, dim1, dim2, dim3] where each dimension has
// n_tokens entries. For text: dim0=linear position, dims1-3=0.
func (e *batchEngine) decodeTextMRoPE(s *slot, tokens []llama.Token) error {
	n := int32(len(tokens))
	if n == 0 {
		return nil
	}

	batch := llama.BatchInit(n, 0, 1)

	// Save original pos pointer so BatchFree doesn't free Go memory.
	origPos := batch.Pos

	// Copy tokens to batch.
	tokenSlice := unsafeSlice(batch.Token, int(n))
	copy(tokenSlice, tokens)

	// Allocate 4D position array for M-RoPE.
	posData := make([]llama.Pos, n*4)
	pos0 := s.nPast
	for i := range n {
		posData[i] = pos0 + llama.Pos(i) // dim 0: linear position
		posData[i+n] = 0                 // dim 1: 0 for text
		posData[i+n*2] = 0               // dim 2: 0 for text
		posData[i+n*3] = 0               // dim 3: 0 for text
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(n))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(n))
	logitsSlice := unsafeSlice(batch.Logits, int(n))

	for i := range n {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if n > 0 {
		logitsSlice[n-1] = 1
	}

	batch.NTokens = n

	e.model.decodeMu.Lock()
	ret, err := llama.Decode(e.model.lctx, batch)
	e.model.decodeMu.Unlock()

	// Restore and free.
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(n)
	return nil
}

// decodeEmbeddingsNormal decodes image embeddings with standard linear positioning.
// Used for non-M-RoPE models where positions are simply sequential integers.
func (e *batchEngine) decodeEmbeddingsNormal(s *slot, embd []float32, nEmbd, nTokens int32) error {
	batch := llama.BatchInit(nTokens, nEmbd, 1)
	defer llama.BatchFree(batch)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	posSlice := unsafeSlice(batch.Pos, int(nTokens))
	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		posSlice[i] = s.nPast + llama.Pos(i)
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}

	batch.NTokens = nTokens

	e.model.decodeMu.Lock()
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, false)
	}
	ret, err := llama.Decode(e.model.lctx, batch)
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, true)
	}
	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(nTokens)
	return nil
}

// decodeEmbeddingsMRoPE decodes image embeddings with M-RoPE 2D positioning.
// For M-RoPE, positions are laid out as 4 contiguous arrays:
//
//	[dim0: n_tokens] [dim1: n_tokens] [dim2: n_tokens] [dim3: n_tokens]
//
// For an image grid of nx columns × ny rows:
//   - dim0 (temporal): pos_0 for all tokens
//   - dim1 (row/y):    pos_0 + y
//   - dim2 (col/x):    pos_0 + x
//   - dim3 (unused):   0
//
// CRITICAL: Position advancement after the image must be max(nx, ny), not n_tokens,
// to avoid position overlap with subsequent text tokens.
func (e *batchEngine) decodeEmbeddingsMRoPE(s *slot, embd []float32, nEmbd, nTokens int32, nx, ny int32) error {
	// For M-RoPE, we need 4x the position slots (4D positions).
	nPosPerEmbd := int32(4)

	batch := llama.BatchInit(nTokens, nEmbd, 1)

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	// Save original pos pointer so BatchFree doesn't try to free Go memory.
	origPos := batch.Pos

	// Allocate our own position array for M-RoPE (4D).
	posData := make([]llama.Pos, nTokens*nPosPerEmbd)

	// Set up 2D M-RoPE positions for image grid.
	pos0 := s.nPast
	for y := range ny {
		for x := range nx {
			i := y*nx + x
			if i >= nTokens {
				break
			}
			// dim 0: constant pos_0 (temporal/base position)
			posData[i] = pos0
			// dim 1: y position (row)
			posData[i+nTokens] = pos0 + llama.Pos(y)
			// dim 2: x position (column)
			posData[i+nTokens*2] = pos0 + llama.Pos(x)
			// dim 3: unused (always 0)
			posData[i+nTokens*3] = 0
		}
	}
	batch.Pos = &posData[0]

	nSeqIDSlice := unsafeSlice(batch.NSeqId, int(nTokens))
	seqIDPtrs := unsafeSlice(batch.SeqId, int(nTokens))
	logitsSlice := unsafeSlice(batch.Logits, int(nTokens))

	for i := range nTokens {
		nSeqIDSlice[i] = 1
		*seqIDPtrs[i] = s.seqID
		logitsSlice[i] = 0
	}

	if nTokens > 0 {
		logitsSlice[nTokens-1] = 1
	}

	batch.NTokens = nTokens

	e.model.decodeMu.Lock()
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, false)
	}

	ret, err := llama.Decode(e.model.lctx, batch)
	if s.useNonCausal {
		llama.SetCausalAttn(e.model.lctx, true)
	}

	e.model.decodeMu.Unlock()

	// Restore original pos pointer before freeing to avoid freeing Go memory.
	batch.Pos = origPos
	llama.BatchFree(batch)

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	// For M-RoPE, n_pos is max(nx, ny) to avoid position overlap.
	nPos := max(ny, nx)
	s.nPast += llama.Pos(nPos)

	return nil
}

// unsafeSlice creates a Go slice from a C pointer. This is used to access
// batch arrays allocated by llama.cpp.
func unsafeSlice[T any](ptr *T, length int) []T {
	if ptr == nil || length <= 0 {
		return nil
	}
	return unsafe.Slice(ptr, length)
}
