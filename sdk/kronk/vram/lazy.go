package vram

import (
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// WeightLoading describes native loading policy and verified platform/device
// support. Zero values retain full-file accounting when support is unknown.
type WeightLoading struct {
	LazyMode          string // "auto" (default), "off", or "on".
	LoadMode          string // "auto" (default), "mmap", "none", "mlock", "mmap+mlock", or "direct-io".
	MMapSupported     bool   // Platform can map the eligible lazy tensor.
	AutoMMapSupported bool   // Every selected device is known to support mmap.
	Metal             bool   // A Metal device may map ordinary weights.
}

// LazyTable contains encoded weight facts, not gathered F32 compute storage.
type LazyTable struct {
	Bytes        int64
	RowBytes     int64
	RowsPerToken int64
	Isolated     bool // Its shard contains only this tensor, avoiding Metal mapping overlap.
}

// LazyTableForShards identifies the table explicitly marked TENSOR_READ_LAZY by
// the supported native architecture. Shard boundaries must not be flattened:
// token_embd.weight can also become a GPU output, so only a table-only shard
// proves isolation. Missing or ambiguous descriptors retain full accounting.
func LazyTableForShards(profile modelprofile.Profile, shards [][]gguf.TensorInfo) LazyTable {
	if profile.LazyRowsPerToken <= 0 {
		return LazyTable{}
	}
	var table LazyTable
	for _, tensors := range shards {
		for _, tensor := range tensors {
			if tensor.Name != "per_layer_token_embd.weight" {
				continue
			}
			if table.Bytes > 0 || len(tensor.Dims) != 2 || tensor.Dims[0] <= 0 || tensor.Dims[1] <= 0 {
				return LazyTable{}
			}
			rowBytes := gguf.GGMLRowSize(tensor.GGMLType, tensor.Dims[0])
			if rowBytes <= 0 || tensor.Bytes != rowBytes*tensor.Dims[1] {
				return LazyTable{}
			}
			table = LazyTable{Bytes: tensor.Bytes, RowBytes: rowBytes, RowsPerToken: profile.LazyRowsPerToken, Isolated: len(tensors) == 1}
		}
	}
	return table
}

// lazyResidentBytes estimates a reclaimable row working set, not a hard native
// cache limit. Reserve two physical batches per context and at least 256 MiB
// for accumulated pages. Random rows can straddle page boundaries; budget an
// extra page per row. Never charge more than the complete table.
func lazyResidentBytes(input Input) int64 {
	table := input.LazyTable
	loading := input.WeightLoading
	if table.Bytes <= 0 || table.RowBytes <= 0 || table.RowsPerToken <= 0 || input.Weights == nil ||
		table.Bytes > input.Weights.InputBytes || table.Bytes > input.ModelSizeBytes || !loading.MMapSupported {
		return table.Bytes
	}
	switch loading.LazyMode {
	case "on":
	case "", "auto":
		if table.Bytes <= 4<<30 || !loading.AutoMMapSupported {
			return table.Bytes
		}
	default:
		return table.Bytes
	}
	switch loading.LoadMode {
	case "none", "direct-io":
	case "", "auto", "mmap":
		if input.ComputeBackend == "" || ((loading.Metal || input.ComputeBackend == "metal") && input.GPULayers != -1 && !table.Isolated) {
			return table.Bytes
		}
	default:
		// mlock can lock intervening lazy pages through a growing prefix.
		return table.Bytes
	}
	page := int64(os.Getpagesize())
	rowPages := (table.RowBytes+page-1)/page + 1
	batch := input.NUBatch
	if batch <= 0 {
		batch = DefaultNUBatch
	}
	workingSet := 2 * batch * table.RowsPerToken * rowPages * page * max(input.ComputeContexts, 1)
	return min(table.Bytes, max(256<<20, workingSet))
}
