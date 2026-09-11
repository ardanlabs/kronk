package models

import (
	"fmt"

	malinavram "github.com/ardanlabs/kronk/sdk/malina/vram"
)

// CalculateVRAM retrieves the installed bundle sizes and computes the VRAM
// requirements for modelID. The pure calculation lives in sdk/malina/vram so
// every caller shares one source of truth.
func (m *Models) CalculateVRAM(modelID string, cfg malinavram.Config) (malinavram.Result, error) {
	path, err := m.FullPath(modelID)
	if err != nil {
		return malinavram.Result{}, fmt.Errorf("calculate-vram: retrieve path modelID[%s]: %w", modelID, err)
	}

	var modelSize int64
	for i, size := range path.FileSizes {
		if size <= 0 {
			return malinavram.Result{}, fmt.Errorf("calculate-vram: modelID[%s]: file[%d]: missing file size", modelID, i)
		}
		modelSize += size
	}

	if modelSize == 0 {
		return malinavram.Result{}, fmt.Errorf("calculate-vram: modelID[%s]: no model files", modelID)
	}

	return malinavram.Calculate(malinavram.Input{
		ModelSizeBytes: modelSize,
		Contexts:       cfg.Contexts,
	}), nil
}
