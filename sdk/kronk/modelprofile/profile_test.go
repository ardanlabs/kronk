package modelprofile

import "testing"

func TestResolveArchitectureParity(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]string
		class      Class
		role       Role
		purpose    Purpose
		memory     MemorySemantics
		mtpLayers  int64
		companion  bool
		audio      bool
		video      bool
		recurrent  int64
		fullLayers int64
	}{
		{
			name: "dense llama",
			metadata: map[string]string{
				"general.architecture": "llama",
				"llama.block_count":    "32",
			},
			class:      ClassDense,
			role:       RoleLanguage,
			purpose:    PurposeGeneration,
			memory:     MemoryAttention,
			fullLayers: 32,
		},
		{
			name: "qwen3 moe",
			metadata: map[string]string{
				"general.architecture":    "qwen3",
				"qwen3.block_count":       "94",
				"qwen3.expert_count":      "128",
				"qwen3.expert_used_count": "8",
			},
			class:      ClassMoE,
			role:       RoleLanguage,
			purpose:    PurposeGeneration,
			memory:     MemoryAttention,
			fullLayers: 94,
		},
		{
			name: "qwen35 hybrid with embedded MTP",
			metadata: map[string]string{
				"general.architecture":              "qwen35moe",
				"qwen35moe.block_count":             "41",
				"qwen35moe.nextn_predict_layers":    "1",
				"qwen35moe.expert_count":            "128",
				"qwen35moe.ssm.conv_kernel":         "4",
				"qwen35moe.ssm.inner_size":          "8",
				"qwen35moe.ssm.state_size":          "4",
				"qwen35moe.ssm.group_count":         "2",
				"qwen35moe.attention.head_count":    "16",
				"qwen35moe.attention.head_count_kv": "2",
			},
			class:      ClassHybrid,
			role:       RoleLanguage,
			purpose:    PurposeGeneration,
			memory:     MemoryRecurrent,
			mtpLayers:  1,
			recurrent:  30,
			fullLayers: 10,
		},
		{
			name: "qwen4 experimental hybrid",
			metadata: map[string]string{
				"general.architecture":             "qwen4exp",
				"qwen4exp.block_count":             "48",
				"qwen4exp.expert_count":            "512",
				"qwen4exp.ssm.conv_kernel":         "4",
				"qwen4exp.ssm.inner_size":          "8",
				"qwen4exp.ssm.state_size":          "4",
				"qwen4exp.ssm.group_count":         "2",
				"qwen4exp.attention.head_count":    "16",
				"qwen4exp.attention.head_count_kv": "2",
			},
			class:      ClassHybrid,
			role:       RoleLanguage,
			purpose:    PurposeGeneration,
			memory:     MemoryRecurrent,
			recurrent:  36,
			fullLayers: 12,
		},
		{
			name: "known recurrent family",
			metadata: map[string]string{
				"general.architecture":  "qwen3next",
				"qwen3next.block_count": "48",
			},
			class:      ClassHybrid,
			role:       RoleLanguage,
			purpose:    PurposeGeneration,
			memory:     MemoryRecurrent,
			fullLayers: 48,
		},
		{
			name: "vision encoder",
			metadata: map[string]string{
				"general.architecture": "qwen2vl",
			},
			class:   ClassDense,
			role:    RoleVisionEncoder,
			purpose: PurposeGeneration,
			memory:  MemoryAttention,
		},
		{
			name: "embedding hint",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"general.basename":     "qwen3-embedding",
			},
			class:   ClassDense,
			role:    RoleLanguage,
			purpose: PurposeEmbedding,
			memory:  MemoryAttention,
		},
		{
			name: "reranker architecture",
			metadata: map[string]string{
				"general.architecture": "bert",
			},
			class:   ClassDense,
			role:    RoleLanguage,
			purpose: PurposeRerank,
			memory:  MemoryAttention,
		},
		{
			name: "omni modalities",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"general.tags":         "[omni]",
			},
			class:   ClassDense,
			role:    RoleLanguage,
			purpose: PurposeGeneration,
			memory:  MemoryAttention,
			audio:   true,
			video:   true,
		},
		{
			name: "architecture modality",
			metadata: map[string]string{
				"general.architecture": "qwen3omni",
			},
			class:   ClassDense,
			role:    RoleLanguage,
			purpose: PurposeGeneration,
			memory:  MemoryAttention,
			audio:   true,
			video:   true,
		},
		{
			name: "shared KV MTP companion",
			metadata: map[string]string{
				"general.architecture":        "gemma4-assistant",
				"gemma4.nextn_predict_layers": "1",
			},
			class:     ClassDense,
			role:      RoleLanguage,
			purpose:   PurposeGeneration,
			memory:    MemoryAttention,
			mtpLayers: 1,
			companion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.metadata)
			if got.Class != tt.class || got.Role != tt.role || got.Purpose != tt.purpose || got.MemorySemantics != tt.memory {
				t.Errorf("profile identity = class %q role %q purpose %q memory %q, want %q %q %q %q",
					got.Class, got.Role, got.Purpose, got.MemorySemantics,
					tt.class, tt.role, tt.purpose, tt.memory)
			}
			if got.Speculation.NextNPredictLayers != tt.mtpLayers || got.Speculation.SharedKVCompanion != tt.companion {
				t.Errorf("speculation = %+v, want layers %d companion %t", got.Speculation, tt.mtpLayers, tt.companion)
			}
			if got.Modalities.Audio != tt.audio || got.Modalities.Video != tt.video {
				t.Errorf("modalities = %+v, want audio %t video %t", got.Modalities, tt.audio, tt.video)
			}
			if got.Attention.RecurrentLayers != tt.recurrent || got.Attention.FullAttentionLayers != tt.fullLayers {
				t.Errorf("attention layers = recurrent %d full %d, want %d/%d",
					got.Attention.RecurrentLayers, got.Attention.FullAttentionLayers, tt.recurrent, tt.fullLayers)
			}
		})
	}
}

func TestResolveQwen35ExplicitRecurrentPattern(t *testing.T) {
	profile := Resolve(map[string]string{
		"general.architecture":              "qwen35",
		"qwen35.block_count":                "5",
		"qwen35.attention.recurrent_layers": "[true false true false true]",
		"qwen35.full_attention_interval":    "2",
		"qwen35.nextn_predict_layers":       "1",
		"qwen35.attention.head_count":       "1",
		"qwen35.attention.head_count_kv":    "1",
		"qwen35.embedding_length":           "8",
		"qwen35.attention.key_length":       "8",
		"qwen35.attention.value_length":     "8",
	})

	if profile.Attention.RecurrentLayers != 2 || profile.Attention.FullAttentionLayers != 2 {
		t.Fatalf("layers: got recurrent=%d full=%d, want 2/2",
			profile.Attention.RecurrentLayers, profile.Attention.FullAttentionLayers)
	}
	if profile.Attention.RecurrentPattern[4] {
		t.Fatal("MTP layer marked recurrent")
	}
}
