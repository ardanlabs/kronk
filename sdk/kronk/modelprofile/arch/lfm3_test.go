package arch_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

func TestLFM3Profile(t *testing.T) {
	for _, tt := range []struct {
		name, heads, pattern string
		recurrent, full, swa int64
		wantErr              bool
	}{
		{"mixed", "[0, 2, 0, 4]", "[false, true, false, false]", 2, 1, 1, false},
		{"recurrent SWA bits do not count as attention", "[0 2 0 4]", "[true true true false]", 2, 1, 1, false},
		{"scalar heads", "2", "[false true false true]", 0, 2, 2, false},
		{"short head array", "[0 2]", "[false true false false]", 0, 0, 0, true},
		{"short SWA array", "[0 2 0 4]", "[false true]", 0, 0, 0, true},
		{"invalid SWA element", "[0 2 0 4]", "[false true bogus false false]", 0, 0, 0, true},
		{"no SWA", "[0 2 0 4]", "[false false false false]", 0, 0, 0, true},
		{"negative heads", "[0 -2 0 4]", "[false true false false]", 0, 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := arch.Resolve(map[string]string{
				"general.architecture": "lfm3",
				"lfm3.block_count":     "4", "lfm3.embedding_length": "256",
				"lfm3.attention.head_count": "8", "lfm3.attention.key_length": "32", "lfm3.attention.value_length": "32",
				"lfm3.attention.head_count_kv": tt.heads, "lfm3.attention.sliding_window_pattern": tt.pattern,
				"lfm3.attention.sliding_window": "512", "lfm3.shortconv.l_cache": "3",
			})
			if err := profile.ValidateForVRAM(); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateForVRAM: got %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			a := profile.Attention
			if a.RecurrentLayers != tt.recurrent || a.FullAttentionLayers != tt.full || a.SlidingWindowLayers != tt.swa {
				t.Errorf("layer counts: got %d/%d/%d, want %d/%d/%d", a.RecurrentLayers, a.FullAttentionLayers, a.SlidingWindowLayers, tt.recurrent, tt.full, tt.swa)
			}
			if tt.recurrent > 0 && (a.RecurrentStateBytes != 2048 || a.FullHeadCountKV != 4 || a.SWAHeadCountKV != 2 || profile.SequenceProcessing != "Hybrid") {
				t.Errorf("hybrid facts: got %+v, classification %q", a, profile.SequenceProcessing)
			}
			if profile.ComputeKind != modelprofile.ComputeGeneric || !profile.TiedOutputFallback {
				t.Errorf("compute/tied output: got %v/%v, want generic/true", profile.ComputeKind, profile.TiedOutputFallback)
			}
			if profile.Batch.InputMixing != modelprofile.BatchInputMixingUnknown || !profile.Speculation.MTPArchitecture.IsZero() {
				t.Errorf("unvalidated capabilities: got batch %+v, speculation %+v", profile.Batch, profile.Speculation)
			}
			if profile.SupportsTensorParallel {
				t.Error("LFM3 must not advertise unsupported tensor parallelism")
			}
		})
	}
}

func TestLFM2ProfileRemainsIndependent(t *testing.T) {
	profile := arch.Resolve(map[string]string{
		"general.architecture": "lfm2",
		"lfm2.block_count":     "4", "lfm2.embedding_length": "256",
		"lfm2.attention.head_count": "8", "lfm2.attention.head_count_kv": "[0 2 0 4]",
		"lfm2.shortconv.l_cache": "3",
	})
	if err := profile.ValidateForVRAM(); err != nil {
		t.Fatal(err)
	}
	if profile.ComputeKind != modelprofile.ComputeLFM2 || profile.Attention.FullAttentionLayers != 2 || profile.Attention.RecurrentStateBytes != 2048 || profile.Attention.SlidingWindowLayers != 0 {
		t.Errorf("LFM2 facts changed: got %+v", profile)
	}
}
