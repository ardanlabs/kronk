package tui

import (
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestModelTraits(t *testing.T) {
	// The facts of real models' headers.
	qwen36 := source.Facts{}
	qwen36.Analysis.Model = models.ModelFacts{BlockCount: 40, MoE: &models.MoEInfo{IsMoE: true, ExpertCount: 256, ExpertUsedCount: 8}}
	qwen36.Analysis.Model.Attention.RecurrentLayers = 30

	gemma := source.Facts{}
	gemma.Analysis.Model = models.ModelFacts{BlockCount: 30, MoE: &models.MoEInfo{IsMoE: true, ExpertCount: 128, ExpertUsedCount: 8}}
	gemma.Analysis.Model.Attention.SlidingWindow, gemma.Analysis.Model.Attention.SlidingWindowLayers = 1024, 25

	dense := source.Facts{}
	dense.Analysis.Model = models.ModelFacts{BlockCount: 28}

	swaUnknown := source.Facts{}
	swaUnknown.Analysis.Model = models.ModelFacts{BlockCount: 30}
	swaUnknown.Analysis.Model.Attention.SlidingWindow = 512

	for name, tt := range map[string]struct {
		f    source.Facts
		want string
	}{
		"Qwen3.6-35B-A3B": {qwen36, "MoE, 8 of 256 experts per token · hybrid: recurrent on 30 of 40 layers"},
		"gemma-4-26B-A4B": {gemma, "MoE, 8 of 128 experts per token · sliding window on 25 of 30 layers"},
		"Qwen3-0.6B":      {dense, "dense"},
		"no layer count":  {swaUnknown, "dense · sliding window"},
	} {
		if got := modelTraits(tt.f); got != tt.want {
			t.Errorf("%s: %q, want %q", name, got, tt.want)
		}
	}
}
