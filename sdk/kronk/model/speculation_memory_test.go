package model

import "testing"

func TestSpeculativeContextCount(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int64
	}{
		{name: "target only", cfg: Config{}, want: 1},
		{name: "separate draft estimated independently", cfg: Config{PtrDraftModel: &DraftModelConfig{ModelFiles: []string{"draft.gguf"}}}, want: 1},
		{name: "companion MTP", cfg: Config{DrafterFile: "mtp.gguf"}, want: 2},
		{name: "disabled", cfg: Config{Speculation: SpeculationDisabled, DrafterFile: "mtp.gguf"}, want: 1},
		{name: "classic ignores companion", cfg: Config{Speculation: SpeculationClassic, DrafterFile: "mtp.gguf"}, want: 1},
		{name: "DSpark execution gated", cfg: Config{Speculation: SpeculationDSpark, DrafterFile: "sidecar.gguf"}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpeculativeContextCount(tt.cfg); got != tt.want {
				t.Errorf("SpeculativeContextCount: got %d, want %d", got, tt.want)
			}
		})
	}
}
