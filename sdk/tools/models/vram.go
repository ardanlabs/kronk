package models

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/vram"
)

// CalculateVRAM retrieves model metadata and computes the VRAM
// requirements for the local model identified by modelID. The pure
// math, file reading, and tensor parsing all live in sdk/kronk/vram so
// every caller (resman planner, post-load model logging, this tools
// helper) shares a single source of truth.
//
// modelID may be a bare model name, "<org>/<model>", "<model>/<variant>",
// or the full "<org>/<model>/<variant>" form — FullPath handles the lookup.
func (m *Models) CalculateVRAM(modelID string, cfg vram.Config) (vram.Result, error) {
	path, err := m.FullPath(modelID)
	if err != nil {
		return vram.Result{}, fmt.Errorf("calculate-vram: failed to retrieve path modelID[%s]: %w", modelID, err)
	}

	return vram.FromFiles(path.ModelFiles, cfg)
}
