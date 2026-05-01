package gguf

import "testing"

func TestDetectMoE(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     MoEInfo
	}{
		{
			name: "qwen3-235b-moe",
			metadata: map[string]string{
				"general.architecture":    "qwen3",
				"qwen3.expert_count":      "128",
				"qwen3.expert_used_count": "8",
				"qwen3.block_count":       "94",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     128,
				ExpertUsedCount: 8,
			},
		},
		{
			name: "deepseek-v3-moe",
			metadata: map[string]string{
				"general.architecture":        "deepseek2",
				"deepseek2.expert_count":      "256",
				"deepseek2.expert_used_count": "8",
				"deepseek2.block_count":       "61",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     256,
				ExpertUsedCount: 8,
			},
		},
		{
			name: "dense-llama",
			metadata: map[string]string{
				"general.architecture": "llama",
				"llama.block_count":    "32",
			},
			want: MoEInfo{
				IsMoE:           false,
				ExpertCount:     0,
				ExpertUsedCount: 0,
			},
		},
		{
			name: "dense-qwen3-8b",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"qwen3.block_count":    "36",
			},
			want: MoEInfo{
				IsMoE:           false,
				ExpertCount:     0,
				ExpertUsedCount: 0,
			},
		},
		{
			name: "fallback-suffix-scan",
			metadata: map[string]string{
				"general.architecture":  "unknown",
				"foo.expert_count":      "64",
				"foo.expert_used_count": "4",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     64,
				ExpertUsedCount: 4,
			},
		},
		{
			name: "shared-experts",
			metadata: map[string]string{
				"general.architecture":          "qwen3",
				"qwen3.expert_count":            "128",
				"qwen3.expert_used_count":       "8",
				"qwen3.ffn_shared_expert_count": "4",
			},
			want: MoEInfo{
				IsMoE:            true,
				ExpertCount:      128,
				ExpertUsedCount:  8,
				HasSharedExperts: true,
			},
		},
		{
			name: "missing-expert-used-count",
			metadata: map[string]string{
				"general.architecture": "mixtral",
				"mixtral.expert_count": "8",
			},
			want: MoEInfo{
				IsMoE:           true,
				ExpertCount:     8,
				ExpertUsedCount: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectMoE(tt.metadata)

			if got.IsMoE != tt.want.IsMoE {
				t.Errorf("IsMoE = %v, want %v", got.IsMoE, tt.want.IsMoE)
			}
			if got.ExpertCount != tt.want.ExpertCount {
				t.Errorf("ExpertCount = %d, want %d", got.ExpertCount, tt.want.ExpertCount)
			}
			if got.ExpertUsedCount != tt.want.ExpertUsedCount {
				t.Errorf("ExpertUsedCount = %d, want %d", got.ExpertUsedCount, tt.want.ExpertUsedCount)
			}
			if got.HasSharedExperts != tt.want.HasSharedExperts {
				t.Errorf("HasSharedExperts = %v, want %v", got.HasSharedExperts, tt.want.HasSharedExperts)
			}
		})
	}
}
