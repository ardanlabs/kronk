package modelprofile_test

import (
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

func TestResolveSpeechPurpose(t *testing.T) {
	decoder := map[string]string{"general.architecture": "clip", "clip.has_gen_audio_encoder": "true"}
	for _, tt := range []struct {
		name      string
		metadata  map[string]string
		projector map[string]string
		want      modelprofile.Purpose
	}{
		{"tagged renamed backbone", map[string]string{"general.architecture": "gpt2", "general.tags": "[text-to-speech speech]"}, nil, modelprofile.PurposeSpeech},
		{"quoted tags", map[string]string{"general.architecture": "gpt2", "general.tags": `["audio", "speech-generation"]`}, nil, modelprofile.PurposeSpeech},
		{"decoder without tags", map[string]string{"general.architecture": "gpt2"}, decoder, modelprofile.PurposeSpeech},
		{"plain gpt2", map[string]string{"general.architecture": "gpt2"}, nil, modelprofile.PurposeGeneration},
		{"speech name is insufficient", map[string]string{"general.architecture": "gpt2", "general.name": "chatterbox-turbo text-to-speech"}, nil, modelprofile.PurposeGeneration},
		{"generic audio tags", map[string]string{"general.architecture": "gpt2", "general.tags": "[audio speech transcription]"}, nil, modelprofile.PurposeGeneration},
		{"tag boundary", map[string]string{"general.architecture": "gpt2", "general.tags": "[not-text-to-speech]"}, nil, modelprofile.PurposeGeneration},
		{"audio input only", map[string]string{"general.architecture": "gpt2"}, map[string]string{"general.architecture": "clip", "clip.has_audio_encoder": "true"}, modelprofile.PurposeGeneration},
		{"generation disabled", map[string]string{"general.architecture": "gpt2"}, map[string]string{"general.architecture": "clip", "clip.has_gen_audio_encoder": "false"}, modelprofile.PurposeGeneration},
		{"invalid generation flag", map[string]string{"general.architecture": "gpt2"}, map[string]string{"general.architecture": "clip", "clip.has_gen_audio_encoder": "invalid"}, modelprofile.PurposeGeneration},
		{"standalone decoder", decoder, nil, modelprofile.PurposeGeneration},
		{"mixed chat retains runtime", map[string]string{"general.architecture": "gpt2", "general.tags": "[text-to-speech]", "tokenizer.chat_template": "chat"}, decoder, modelprofile.PurposeGeneration},
		{"embedding retains purpose", map[string]string{"general.architecture": "bert", "bert.pooling_type": "1", "general.tags": "[text-to-speech]"}, decoder, modelprofile.PurposeEmbedding},
		{"decision retains purpose", map[string]string{"general.architecture": "modern-bert", "modern-bert.decision.type": "laya", "general.tags": "[text-to-speech]"}, decoder, modelprofile.PurposeGeneration},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := arch.ResolveBundle(tt.metadata, tt.projector, "renamed")
			if profile.Purpose != tt.want {
				t.Errorf("purpose: got %s, want %s", profile.Purpose, tt.want)
			}
		})
	}
}

func TestResolveRequiresFileMetadata(t *testing.T) {
	for _, tt := range []struct {
		architecture string
		want         bool
	}{
		{"lfm3", true},
		{"kimi-k3", true},
		{"kimi-k31", true},
		{"lfm2", false},
		{"llama", false},
		{"unknown", false},
	} {
		for _, blocks := range []string{"0", "4"} {
			t.Run(tt.architecture+"/blocks-"+blocks, func(t *testing.T) {
				// Scalar-only native metadata must advertise the requirement
				// even when missing arrays cause validation to fail or return early.
				profile := arch.Resolve(map[string]string{
					"general.architecture":           tt.architecture,
					tt.architecture + ".block_count": blocks,
				})
				if profile.RequiresFileMetadata != tt.want {
					t.Errorf("RequiresFileMetadata: got %v, want %v", profile.RequiresFileMetadata, tt.want)
				}
			})
		}
	}
}

func TestResolveArchitectureParity(t *testing.T) {
	tests := []struct {
		name       string
		metadata   map[string]string
		moe        bool
		role       modelprofile.Role
		purpose    modelprofile.Purpose
		memory     modelprofile.MemorySemantics
		mtpLayers  int64
		mtpArch    modelprofile.MTPArchitecture
		companion  bool
		ownKV      bool
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
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryAttention,
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
			moe:        true,
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryAttention,
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
			moe:        true,
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryRecurrent,
			mtpLayers:  1,
			mtpArch:    modelprofile.MTPArchitectureQwen35OwnKV,
			ownKV:      true,
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
			moe:        true,
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryRecurrent,
			recurrent:  36,
			fullLayers: 12,
		},
		{
			name: "Nemotron 3 Super hybrid with embedded MTP",
			metadata: map[string]string{
				"general.architecture":                   "nemotron_h_moe",
				"nemotron_h_moe.block_count":             "88",
				"nemotron_h_moe.nextn_predict_layers":    "1",
				"nemotron_h_moe.expert_count":            "128",
				"nemotron_h_moe.expert_used_count":       "4",
				"nemotron_h_moe.attention.head_count":    "32",
				"nemotron_h_moe.attention.head_count_kv": "8",
				"nemotron_h_moe.attention.key_length":    "128",
				"nemotron_h_moe.attention.value_length":  "128",
				"nemotron_h_moe.ssm.state_size":          "128",
				"nemotron_h_moe.ssm.conv_kernel":         "4",
				"nemotron_h_moe.ssm.inner_size":          "8192",
				"nemotron_h_moe.ssm.group_count":         "8",
			},
			moe:        true,
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryRecurrent,
			mtpLayers:  1,
			mtpArch:    modelprofile.MTPArchitectureNemotronOwnKV,
			ownKV:      true,
			fullLayers: 88,
		},
		{
			name: "known recurrent family",
			metadata: map[string]string{
				"general.architecture":  "qwen3next",
				"qwen3next.block_count": "48",
			},
			role:       modelprofile.RoleLanguage,
			purpose:    modelprofile.PurposeGeneration,
			memory:     modelprofile.MemoryRecurrent,
			fullLayers: 48,
		},
		{
			name: "vision encoder",
			metadata: map[string]string{
				"general.architecture": "qwen2vl",
			},
			role:    modelprofile.RoleVisionEncoder,
			purpose: modelprofile.PurposeGeneration,
			memory:  modelprofile.MemoryAttention,
		},
		{
			name: "embedding hint",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"general.basename":     "qwen3-embedding",
			},
			role:    modelprofile.RoleLanguage,
			purpose: modelprofile.PurposeEmbedding,
			memory:  modelprofile.MemoryAttention,
		},
		{
			name: "BERT embedding default",
			metadata: map[string]string{
				"general.architecture": "bert",
			},
			role:    modelprofile.RoleLanguage,
			purpose: modelprofile.PurposeEmbedding,
			memory:  modelprofile.MemoryAttention,
		},
		{
			name: "omni modalities",
			metadata: map[string]string{
				"general.architecture": "qwen3",
				"general.tags":         "[omni]",
			},
			role:    modelprofile.RoleLanguage,
			purpose: modelprofile.PurposeGeneration,
			memory:  modelprofile.MemoryAttention,
			audio:   true,
			video:   true,
		},
		{
			name: "architecture modality",
			metadata: map[string]string{
				"general.architecture": "qwen3omni",
			},
			role:    modelprofile.RoleLanguage,
			purpose: modelprofile.PurposeGeneration,
			memory:  modelprofile.MemoryAttention,
			audio:   true,
			video:   true,
		},
		{
			name: "shared KV MTP companion",
			metadata: map[string]string{
				"general.architecture":        "gemma4-assistant",
				"gemma4.nextn_predict_layers": "1",
			},
			role:      modelprofile.RoleLanguage,
			purpose:   modelprofile.PurposeGeneration,
			memory:    modelprofile.MemoryAttention,
			mtpLayers: 1,
			mtpArch:   modelprofile.MTPArchitectureGemmaSharedKV,
			companion: true,
		},
		{
			name: "own KV MTP companion",
			metadata: map[string]string{
				"general.architecture":        "qwen35",
				"qwen35.nextn_predict_layers": "1",
			},
			role:      modelprofile.RoleLanguage,
			purpose:   modelprofile.PurposeGeneration,
			memory:    modelprofile.MemoryRecurrent,
			mtpLayers: 1,
			mtpArch:   modelprofile.MTPArchitectureQwen35OwnKV,
			ownKV:     true,
		},
		{
			name: "qwen4exp own KV MTP companion",
			metadata: map[string]string{
				"general.architecture":          "qwen4exp",
				"qwen4exp.nextn_predict_layers": "1",
			},
			role:      modelprofile.RoleLanguage,
			purpose:   modelprofile.PurposeGeneration,
			memory:    modelprofile.MemoryRecurrent,
			mtpLayers: 1,
			mtpArch:   modelprofile.MTPArchitectureQwen4ExpOwnKV,
			ownKV:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := arch.Resolve(tt.metadata)
			if got.MoE.IsMoE != tt.moe || got.Role != tt.role || got.Purpose != tt.purpose || got.MemorySemantics != tt.memory {
				t.Errorf("profile identity = moe %t role %q purpose %q memory %q, want %t %q %q %q",
					got.MoE.IsMoE, got.Role, got.Purpose, got.MemorySemantics,
					tt.moe, tt.role, tt.purpose, tt.memory)
			}
			if got.Speculation.NextNPredictLayers != tt.mtpLayers || got.Speculation.MTPArchitecture != tt.mtpArch || got.Speculation.SharedKVCompanion != tt.companion || got.Speculation.OwnKVCompanion != tt.ownKV {
				t.Errorf("speculation = %+v, want layers %d architecture %q shared companion %t own-KV companion %t", got.Speculation, tt.mtpLayers, tt.mtpArch, tt.companion, tt.ownKV)
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

func TestMTPEnabled(t *testing.T) {
	for _, architecture := range []modelprofile.MTPArchitecture{
		modelprofile.MTPArchitectureQwen35OwnKV,
		modelprofile.MTPArchitectureQwen4ExpOwnKV,
		modelprofile.MTPArchitectureNemotronOwnKV,
		modelprofile.MTPArchitectureGemmaSharedKV,
	} {
		if !modelprofile.MTPEnabled(architecture) {
			t.Errorf("MTPEnabled(%q) = false, want true", architecture)
		}
	}
	if modelprofile.MTPEnabled(modelprofile.MTPArchitectureGLM5Next) {
		t.Fatal("MTPEnabled(GLM5Next) = true, want false")
	}
}

func TestResolveGLM5Next(t *testing.T) {
	headCountKV := "[" + strings.TrimSpace(strings.Repeat("0 ", 34)+strings.Repeat("1 ", 12)) + "]"
	profile := arch.Resolve(map[string]string{
		"general.architecture":              "glm5-next",
		"glm5-next.block_count":             "46",
		"glm5-next.embedding_length":        "6144",
		"glm5-next.expert_count":            "256",
		"glm5-next.nextn_predict_layers":    "1",
		"glm5-next.attention.head_count":    "64",
		"glm5-next.attention.head_count_kv": headCountKV,
		"glm5-next.attention.key_length":    "512",
		"glm5-next.attention.value_length":  "512",
		"glm5-next.ssm.conv_kernel":         "4",
		"glm5-next.kda.head_dim":            "128",
	})

	if profile.MemorySemantics != modelprofile.MemoryRecurrent || !profile.MoE.IsMoE {
		t.Fatalf("identity = memory %q moe %t, want recurrent/true",
			profile.MemorySemantics, profile.MoE.IsMoE)
	}
	if profile.Attention.RecurrentLayers != 34 || profile.Attention.FullAttentionLayers != 11 {
		t.Fatalf("layers = recurrent %d full %d, want 34/11",
			profile.Attention.RecurrentLayers, profile.Attention.FullAttentionLayers)
	}
	if len(profile.Attention.RecurrentPattern) != 46 || profile.Attention.RecurrentPattern[45] {
		t.Fatalf("recurrent pattern does not exclude the appended NextN layer: %v", profile.Attention.RecurrentPattern)
	}
	if profile.Attention.FullHeadCountKV != 1 {
		t.Errorf("FullHeadCountKV = %d, want 1", profile.Attention.FullHeadCountKV)
	}
	if profile.Attention.RecurrentStateBytes != 4_489_216 {
		t.Errorf("RecurrentStateBytes = %d, want 4489216", profile.Attention.RecurrentStateBytes)
	}
	if profile.Speculation.MTPArchitecture != modelprofile.MTPArchitectureGLM5Next {
		t.Errorf("MTPArchitecture = %q, want %q", profile.Speculation.MTPArchitecture, modelprofile.MTPArchitectureGLM5Next)
	}
	if err := profile.ValidateForVRAM(); err != nil {
		t.Fatalf("ValidateForVRAM() error = %v", err)
	}
}

func TestResolveQwen35ExplicitRecurrentPattern(t *testing.T) {
	profile := arch.Resolve(map[string]string{
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
	if profile.ComputeKind != modelprofile.ComputeQwen35 {
		t.Errorf("ComputeKind: got %d, want %d", profile.ComputeKind, modelprofile.ComputeQwen35)
	}
}
