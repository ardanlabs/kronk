package mtp

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// SyncInput describes target rows that must be synchronized into the MTP view.
type SyncInput struct {
	Tokens         []llama.Token
	HiddenRows     []float32
	PendingHidden  []float32
	HiddenScratch  []float32
	BasePosition   llama.Pos
	EmbeddingSize  int
	ChunkSize      int
	SharedKV       bool
	DecodeOwnChunk func(tokens []llama.Token, basePosition llama.Pos, hiddenRows []float32) error
}

// SyncResult contains MTP state after target synchronization.
type SyncResult struct {
	PendingHidden []float32
	Position      llama.Pos
	Active        bool
}

// Synchronize advances the MTP view after target decoding.
func Synchronize(input SyncInput) (SyncResult, error) {
	nTokens := len(input.Tokens)
	if nTokens == 0 {
		return SyncResult{}, nil
	}
	if input.EmbeddingSize <= 0 || len(input.HiddenRows) != nTokens*input.EmbeddingSize {
		return SyncResult{}, fmt.Errorf("synchronizing MTP: got %d hidden values for %d tokens with embedding size %d", len(input.HiddenRows), nTokens, input.EmbeddingSize)
	}

	if !input.SharedKV {
		if input.ChunkSize <= 0 {
			return SyncResult{}, fmt.Errorf("synchronizing MTP: invalid chunk size %d", input.ChunkSize)
		}
		scratch := input.HiddenScratch
		need := min(input.ChunkSize, nTokens) * input.EmbeddingSize
		if cap(scratch) < need {
			scratch = make([]float32, need)
		}

		for start := 0; start < nTokens; start += input.ChunkSize {
			end := min(start+input.ChunkSize, nTokens)
			hidden := scratch[:(end-start)*input.EmbeddingSize]
			for i := start; i < end; i++ {
				dst := hidden[(i-start)*input.EmbeddingSize : (i-start+1)*input.EmbeddingSize]
				switch {
				case i == 0 && len(input.PendingHidden) == input.EmbeddingSize:
					copy(dst, input.PendingHidden)
				case i == 0:
					clear(dst)
				default:
					copy(dst, input.HiddenRows[(i-1)*input.EmbeddingSize:i*input.EmbeddingSize])
				}
			}
			if err := input.DecodeOwnChunk(input.Tokens[start:end], input.BasePosition+llama.Pos(start), hidden); err != nil {
				return SyncResult{}, err
			}
		}
	}

	pending := input.PendingHidden
	if cap(pending) < input.EmbeddingSize {
		pending = make([]float32, input.EmbeddingSize)
	} else {
		pending = pending[:input.EmbeddingSize]
	}
	copy(pending, input.HiddenRows[(nTokens-1)*input.EmbeddingSize:])

	return SyncResult{PendingHidden: pending, Position: input.BasePosition + llama.Pos(nTokens), Active: true}, nil
}
