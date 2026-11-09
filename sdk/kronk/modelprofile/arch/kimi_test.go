package arch_test

import (
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

func TestKimiK31Profile(t *testing.T) {
	for _, tt := range []struct {
		name, heads, pattern, topK string
		wantErr                    bool
	}{
		{"hybrid shared indexer", "[0 1 0 1 1]", "[true true false false true]", "64", false},
		{"default full indexers", "[0 1 0 1 1]", "", "64", false},
		{"short KV heads", "[0 1]", "", "64", true},
		{"negative KV heads", "[0 1 -1 1 1]", "", "64", true},
		{"short indexer pattern", "[0 1 0 1 1]", "[true false]", "64", true},
		{"missing source", "[0 1 0 1 1]", "false", "64", true},
		{"missing top k", "[0 1 0 1 1]", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			metadata := map[string]string{
				"general.architecture": "kimi-k31",
				"kimi-k31.block_count": "5", "kimi-k31.embedding_length": "256",
				"kimi-k31.attention.head_count": "8", "kimi-k31.attention.head_count_kv": tt.heads,
				"kimi-k31.attention.key_length": "64", "kimi-k31.attention.kv_lora_rank": "32",
				"kimi-k31.ssm.conv_kernel": "3", "kimi-k31.kda.head_dim": "16",
				"kimi-k31.attention.indexer.key_length": "32", "kimi-k31.attention.indexer.head_count": "4",
				"kimi-k31.attention.q_lora_rank": "64", "kimi-k31.expert_shared_count": "1",
			}
			if tt.pattern != "" {
				metadata["kimi-k31.attention.indexer.types"] = tt.pattern
			}
			if tt.topK != "" {
				metadata["kimi-k31.attention.indexer.top_k"] = tt.topK
			}
			profile := arch.Resolve(metadata)
			if err := profile.ValidateForVRAM(); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateForVRAM: got %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if profile.ComputeKind != modelprofile.ComputeKimiK3 {
				t.Errorf("compute kind: got %v, want %v", profile.ComputeKind, modelprofile.ComputeKimiK3)
			}
			a := profile.Attention
			if profile.Dimensions.HeadCountKV != 1 || profile.Dimensions.ValueLength != 32 || a.RecurrentLayers != 2 || a.FullAttentionLayers != 3 || a.RecurrentStateBytes != 11264 {
				t.Errorf("KDA/MLA facts: got %+v / %+v", profile.Dimensions, a)
			}
			want := []int64{-1, 1, -1, 1, 4}
			if tt.pattern == "" {
				want[3] = 3
			}
			if !slices.Equal(a.IndexerSources, want) {
				t.Errorf("indexer owners: got %v, want %v", a.IndexerSources, want)
			}
			if profile.SequenceProcessing != "Hybrid" || profile.SupportsTensorParallel || !profile.MoE.HasSharedExperts || !profile.Speculation.MTPArchitecture.IsZero() {
				t.Errorf("Kimi capabilities: got %+v", profile)
			}
		})
	}
}
