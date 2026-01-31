package catalog

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// CalculateVRAM retrieves model metadata and computes the VRAM requirements.
func (c *Catalog) CalculateVRAM(modelID string, mc ModelConfig) (models.VRAM, error) {
	var cacheSequences int64
	if mc.SystemPromptCache || mc.IncrementalCache {
		cacheSequences = 1
	}

	cfg := models.VRAMConfig{
		ContextWindow:   int64(mc.ContextWindow),
		BytesPerElement: ggmlTypeToBytes(mc.CacheTypeK, mc.CacheTypeV),
		Slots:           int64(mc.NSeqMax),
		CacheSequences:  cacheSequences,
	}

	vram, err := c.models.CalculateVRAM(modelID, cfg)
	if err != nil {
		return models.VRAM{}, fmt.Errorf("calculate-vram: unable to get model details: %w", err)
	}

	return vram, nil
}

// =============================================================================

func ggmlTypeToBytes(typeK, typeV model.GGMLType) int64 {
	bytesK := ggmlBytes(typeK)
	bytesV := ggmlBytes(typeV)

	if bytesK > bytesV {
		return bytesK
	}
	return bytesV
}

func ggmlBytes(t model.GGMLType) int64 {
	switch t {
	case model.GGMLTypeF32:
		return 4
	case model.GGMLTypeF16, model.GGMLTypeBF16:
		return 2
	case model.GGMLTypeQ8_0:
		return 1
	case model.GGMLTypeQ4_0, model.GGMLTypeQ4_1, model.GGMLTypeQ5_0, model.GGMLTypeQ5_1:
		return 1 // Conservatively round up from 0.5-0.625
	default:
		return 2 // Default to f16 for auto/unknown
	}
}
