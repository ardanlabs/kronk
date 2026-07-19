package model

import (
	"fmt"
	"runtime"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// =============================================================================
// MTMD BATCH DECODE HELPERS
// =============================================================================

// decodeTextMRoPE decodes text tokens for M-RoPE models.
// M-RoPE uses 4D positions: [dim0, dim1, dim2, dim3] where each dimension has
// n_tokens entries. Text uses the same logical position in all four planes.
func (e *batchEngine) decodeTextMRoPE(s *slot, tokens []llama.Token) error {
	n := int32(len(tokens))
	if n == 0 {
		return nil
	}

	batch := e.mropeBatch

	// Copy tokens to batch.
	tokenSlice := unsafeSlice(batch.Token, int(n))
	copy(tokenSlice, tokens)

	// Fill 4D position array for M-RoPE using pre-allocated buffer.
	posData := e.mropePosData[:n*4]
	pos0 := s.nPast
	fillMRoPETextPositions(posData, n, pos0)
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
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
	}
	e.model.decodeMu.Unlock()

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
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
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
//   - dim0 (temporal): pos_0
//   - dim1 (row/y):   pos_0 + y
//   - dim2 (col/x):   pos_0 + x
//   - dim3 (unused):  0
func (e *batchEngine) decodeEmbeddingsMRoPE(s *slot, embd []float32, nEmbd, nTokens int32, nx, ny int32) error {
	if nTokens != nx*ny {
		return fmt.Errorf("mrope image layout: unsupported token count %d for grid %dx%d", nTokens, nx, ny)
	}

	// For M-RoPE, we need 4x the position slots (4D positions).
	nPosPerEmbd := int32(4)

	batch := llama.BatchInit(nTokens, nEmbd, 1)

	// Save original pos pointer so BatchFree doesn't try to free Go memory.
	origPos := batch.Pos
	defer func() {
		batch.Pos = origPos
		llama.BatchFree(batch)
	}()

	embdSlice := unsafeSlice(batch.Embd, int(nTokens*nEmbd))
	copy(embdSlice, embd)

	// Allocate our own position array for M-RoPE (4D).
	posData := make([]llama.Pos, nTokens*nPosPerEmbd)

	pos0 := s.nPast
	fillMRoPEImagePositions(posData, nTokens, nx, ny, pos0)
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
	if err == nil && ret == 0 {
		llama.Synchronize(e.model.lctx)
	}
	runtime.KeepAlive(posData)

	e.model.decodeMu.Unlock()

	if err != nil || ret != 0 {
		return decodeError(ret, err)
	}

	s.nPast += llama.Pos(max(nx, ny))

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

func fillMRoPEImagePositions(positions []llama.Pos, nTokens, nx, ny int32, start llama.Pos) {
	for y := range ny {
		for x := range nx {
			i := y*nx + x
			if i >= nTokens {
				break
			}
			positions[i] = start
			positions[i+nTokens] = start + llama.Pos(y)
			positions[i+nTokens*2] = start + llama.Pos(x)
			positions[i+nTokens*3] = 0
		}
	}
}

// unsafeSlice creates a Go slice from a C pointer. This is used to access
// batch arrays allocated by llama.cpp.
func unsafeSlice[T any](ptr *T, length int) []T {
	if ptr == nil || length <= 0 {
		return nil
	}
	return unsafe.Slice(ptr, length)
}
