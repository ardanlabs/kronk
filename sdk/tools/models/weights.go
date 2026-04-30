package models

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// WeightBreakdown provides per-category weight size information. Field
// shape mirrors gguf.WeightBreakdown so the two are interchangeable;
// this models-side type lets the package keep its public API stable
// without leaking the gguf import in struct fields.
type WeightBreakdown struct {
	TotalBytes         int64
	AlwaysActiveBytes  int64
	ExpertBytesTotal   int64
	ExpertBytesByLayer []int64
}

// weightBreakdownFromGGUF converts a gguf.WeightBreakdown into the
// models-side type. Returns nil for the zero-tensor case so callers can
// keep using a *WeightBreakdown pointer field.
func weightBreakdownFromGGUF(w gguf.WeightBreakdown) WeightBreakdown {
	return WeightBreakdown{
		TotalBytes:         w.TotalBytes,
		AlwaysActiveBytes:  w.AlwaysActiveBytes,
		ExpertBytesTotal:   w.ExpertBytesTotal,
		ExpertBytesByLayer: w.ExpertBytesByLayer,
	}
}

// fetchGGUFHeaderAndTensors fetches GGUF header, KV metadata, and tensor
// descriptors from a remote URL using HTTP Range requests. Only the
// header sections are downloaded, not the actual tensor data. Parsing is
// delegated to sdk/kronk/gguf.
func fetchGGUFHeaderAndTensors(ctx context.Context, url string) (metadata map[string]string, tensors []gguf.TensorInfo, fileSize int64, err error) {
	data, fileSize, err := gguf.FetchHeaderBytes(ctx, url)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to fetch header data: %w", err)
	}

	metadata, tensors, err = gguf.ParseHeaderAndTensors(data, fileSize)
	if err != nil {
		return nil, nil, 0, err
	}

	return metadata, tensors, fileSize, nil
}
