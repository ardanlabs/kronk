package arch_test

import (
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

func TestInklingProfile(t *testing.T) {
	for _, tt := range []struct {
		name, key, value string
		wantErr          bool
	}{
		{"loop", "", "", false},
		{"single variable pass", "loop.count", "1", true},
		{"missing gate rank", "loop.gate_rank", "", true},
		{"empty core", "loop.end", "1", true},
		{"unaligned core", "attn_res.block_size", "2", true},
		{"too many slots", "loop.count", "256", true},
		{"invalid varying flag", "loop.varying", "invalid", true},
		{"zero head count", "attention.head_count_kv", "[1 2 0 1]", true},
		{"incompatible heads", "attention.head_count_kv", "[1 3 4 1]", true},
		{"short head array", "attention.head_count", "[4 8]", true},
		{"invalid SWA element", "attention.sliding_window_pattern", "[false true bogus false true]", true},
		{"short SWA pattern", "attention.sliding_window_pattern", "[false true]", true},
		{"missing convolution window", "shortconv.l_cache", "", true},
		{"oversized convolution window", "shortconv.l_cache", "65", true},
		{"NextN is unsupported", "nextn_predict_layers", "1", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture":      "inkling2_loop",
				"inkling2_loop.block_count": "4", "inkling2_loop.embedding_length": "128",
				"inkling2_loop.attention.head_count": "[4 8 8 4]", "inkling2_loop.attention.head_count_kv": "[1 2 4 1]",
				"inkling2_loop.attention.key_length": "8", "inkling2_loop.attention.value_length": "8",
				"inkling2_loop.attention.sliding_window": "128", "inkling2_loop.attention.sliding_window_pattern": "[false true false true]",
				"inkling2_loop.shortconv.l_cache": "3", "inkling2_loop.attn_res.block_size": "1",
				"inkling2_loop.loop.start": "1", "inkling2_loop.loop.end": "3", "inkling2_loop.loop.count": "3", "inkling2_loop.loop.gate_rank": "8",
			}
			if tt.key != "" {
				metadata["inkling2_loop."+tt.key] = tt.value
			}
			p := arch.Resolve(metadata)
			if err := p.ValidateForVRAM(); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateForVRAM: got %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if p.Dimensions.BlockCount != 4 || p.Dimensions.HeadCount != 8 || p.Dimensions.HeadCountKV != 4 {
				t.Errorf("physical dimensions: got %+v", p.Dimensions)
			}
			a := p.Attention
			if !slices.Equal(a.HeadCountKV, []int64{1, 2, 4, 1, 2, 4, 2, 4}) ||
				!slices.Equal(a.SWAPattern, []bool{false, true, false, true, true, false, true, false}) {
				t.Errorf("cache-slot topology: got heads %v, SWA %v", a.HeadCountKV, a.SWAPattern)
			}
			if a.RecurrentStateBytes != 2560 || a.RecurrentLayers != 8 || a.FullAttentionLayers != 4 || a.SlidingWindowLayers != 4 || a.RecurrentPattern != nil {
				t.Errorf("coexisting convolution and attention: got %+v", a)
			}
			if p.MemorySemantics != modelprofile.MemoryRecurrent || p.SequenceProcessing != "Hybrid" || !p.RequiresFileMetadata || p.ComputeKind != modelprofile.ComputeInkling {
				t.Errorf("runtime semantics: got %+v", p)
			}
			if p.SupportsTensorParallel || p.Batch.InputMixing != modelprofile.BatchInputMixingUnsupported || !p.Speculation.MTPArchitecture.IsZero() {
				t.Errorf("unsupported capabilities were enabled: got %+v", p)
			}
		})
	}
}
