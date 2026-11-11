package mtp

import (
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// SyncInput describes a completed target contribution. The host retains its
// token/embedding rows and explicit positions; MTP owns the hidden-state shift.
// NextPosition is the logical continuation position, not an embedding-row count.
// The host's rows and HiddenRows must remain valid until Synchronize returns.
type SyncInput struct {
	RowCount      int
	HiddenRows    []float32
	PendingHidden []float32
	HiddenScratch []float32
	NextPosition  llama.Pos
	EmbeddingSize int
	ChunkSize     int
	SharedKV      bool
	Device        *DeviceSync
	// DecodeOwnChunk replays the host's rows in [start,end) with shifted
	// hidden states. Shared-KV handoff never invokes this callback.
	DecodeOwnChunk func(start, end int, hiddenRows []float32) error
}

// DeviceSync borrows the target's last-decode rows for own-KV replay. It must
// be consumed before another target decode. Only the final row is copied to
// host memory, where pending state survives cache snapshots and target rollback.
type DeviceSync struct {
	DecodeChunk func(start, end int) error
	ReadLastRow func(out []float32) error
}

// SyncResult contains MTP state after target synchronization.
type SyncResult struct {
	PendingHidden []float32
	Position      llama.Pos
	Active        bool
}

// Synchronize advances the MTP view after target decoding.
func Synchronize(input SyncInput) (SyncResult, error) {
	nRows := input.RowCount
	if nRows == 0 {
		return SyncResult{}, nil
	}
	if nRows < 0 || input.EmbeddingSize <= 0 || (input.Device == nil && len(input.HiddenRows) != nRows*input.EmbeddingSize) {
		return SyncResult{}, fmt.Errorf("synchronizing MTP: got %d hidden values for %d rows with embedding size %d", len(input.HiddenRows), nRows, input.EmbeddingSize)
	}

	if !input.SharedKV {
		if input.ChunkSize <= 0 {
			return SyncResult{}, fmt.Errorf("synchronizing MTP: invalid chunk size %d", input.ChunkSize)
		}
		scratch := input.HiddenScratch
		need := min(input.ChunkSize, nRows) * input.EmbeddingSize
		if input.Device == nil && cap(scratch) < need {
			scratch = make([]float32, need)
		}

		for start := 0; start < nRows; start += input.ChunkSize {
			end := min(start+input.ChunkSize, nRows)
			if input.Device != nil {
				if err := input.Device.DecodeChunk(start, end); err != nil {
					return SyncResult{}, err
				}
				continue
			}
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
			if err := input.DecodeOwnChunk(start, end, hidden); err != nil {
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
	if input.Device != nil {
		if err := input.Device.ReadLastRow(pending); err != nil {
			return SyncResult{}, err
		}
	} else {
		copy(pending, input.HiddenRows[(nRows-1)*input.EmbeddingSize:])
	}

	return SyncResult{PendingHidden: pending, Position: input.NextPosition, Active: true}, nil
}
