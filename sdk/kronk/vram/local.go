package vram

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// FromFiles computes the VRAM requirements for a model whose GGUF files
// are already on the local disk. paths must list every shard so the
// total on-disk size is correct; metadata and tensor descriptors are
// read from paths[0].
//
// This is the single entry point for any caller that has the model
// stored locally (resman planner, post-load model logging, models
// catalog operations) so the slot-memory and weight-breakdown math is
// produced in exactly one place.
func FromFiles(paths []string, cfg Config) (Result, error) {
	if len(paths) == 0 {
		return Result{}, fmt.Errorf("from-files: no model files provided")
	}

	var totalSize int64
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return Result{}, fmt.Errorf("from-files: failed to stat %s: %w", p, err)
		}
		totalSize += info.Size()
	}

	data, err := gguf.ReadHeaderBytes(paths[0])
	if err != nil {
		return Result{}, fmt.Errorf("from-files: %w", err)
	}

	return FromBytes(data, totalSize, cfg)
}
