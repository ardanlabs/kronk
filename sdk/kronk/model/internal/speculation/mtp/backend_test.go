package mtp

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestMTPBackendForPlan(t *testing.T) {
	tests := []struct {
		name              string
		plan              speculation.Plan
		wantName          string
		wantSharedKV      bool
		wantFixedPosition bool
		wantWideEmbedding bool
		wantErr           bool
	}{
		{"embedded Qwen35", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureQwen35OwnKV, MTPArtifact: speculation.MTPArtifactEmbedded}, "qwen35-own-kv", false, false, false, false},
		{"companion Qwen35", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureQwen35OwnKV, MTPArtifact: speculation.MTPArtifactCompanion}, "qwen35-own-kv", false, false, false, false},
		{"embedded Qwen4Exp", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureQwen4ExpOwnKV, MTPArtifact: speculation.MTPArtifactEmbedded}, "qwen4exp-own-kv", false, false, true, false},
		{"companion Qwen4Exp", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureQwen4ExpOwnKV, MTPArtifact: speculation.MTPArtifactCompanion}, "qwen4exp-own-kv", false, false, true, false},
		{"embedded Nemotron", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureNemotronOwnKV, MTPArtifact: speculation.MTPArtifactEmbedded}, "nemotron-own-kv", false, false, false, false},
		{"companion Nemotron rejected", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureNemotronOwnKV, MTPArtifact: speculation.MTPArtifactCompanion}, "", false, false, false, true},
		{"companion Gemma", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureGemmaSharedKV, MTPArtifact: speculation.MTPArtifactCompanion}, "gemma-shared-kv", true, true, false, false},
		{"embedded Gemma rejected", speculation.Plan{MTPArchitecture: speculation.MTPArchitectureGemmaSharedKV, MTPArtifact: speculation.MTPArtifactEmbedded}, "", false, false, false, true},
		{"missing architecture rejected", speculation.Plan{MTPArtifact: speculation.MTPArtifactCompanion}, "", false, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := BackendForPlan(tt.plan)
			if (err != nil) != tt.wantErr {
				t.Fatalf("BackendForPlan() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got := backend.Name(); got != tt.wantName {
				t.Errorf("name = %q, want %q", got, tt.wantName)
			}
			if backend.artifact != tt.plan.MTPArtifact {
				t.Errorf("artifact = %s, want %s", backend.artifact, tt.plan.MTPArtifact)
			}
			if backend.wideEmbedding != tt.wantWideEmbedding {
				t.Errorf("wideEmbedding = %t, want %t", backend.wideEmbedding, tt.wantWideEmbedding)
			}
			if got := backend.SharedKV(); got != tt.wantSharedKV {
				t.Errorf("sharedKV = %t, want %t", got, tt.wantSharedKV)
			}
			if got := backend.FixedDraftPosition(); got != tt.wantFixedPosition {
				t.Errorf("fixedDraftPosition = %t, want %t", got, tt.wantFixedPosition)
			}
		})
	}
}

func TestEmbeddedMTPContextParamsCapsOutputs(t *testing.T) {
	target := llama.ContextParams{
		NCtx:               4096,
		NBatch:             2064,
		NUbatch:            2064,
		NSeqMax:            4,
		NRsSeq:             3,
		NThreads:           6,
		NThreadsBatch:      8,
		FlashAttentionType: llama.FlashAttentionTypeAuto,
		TypeK:              llama.GGMLTypeF16,
		TypeV:              llama.GGMLTypeF16,
		Offload_kqv:        1,
		OpOffload:          1,
		KVUnified:          1,
		SwaFull:            1,
		RopeScalingType:    llama.RopeScalingTypeYARN,
		RopeFreqBase:       1_000_000,
		RopeFreqScale:      0.25,
		YarnExtFactor:      1.5,
		YarnAttnFactor:     1.25,
		YarnBetaFast:       32,
		YarnBetaSlow:       1,
		YarnOrigCtx:        32_768,
	}

	got := ownKVContextParams(llama.ContextParams{NRsSeq: 7}, target)
	if got.CtxType != llama.ContextTypeMTP {
		t.Errorf("CtxType = %d, want %d", got.CtxType, llama.ContextTypeMTP)
	}
	if got.NRsSeq != 0 {
		t.Errorf("NRsSeq = %d, want 0 for the MTP draft context", got.NRsSeq)
	}
	if got.NOutputsMax != target.NSeqMax {
		t.Errorf("NOutputsMax = %d, want %d", got.NOutputsMax, target.NSeqMax)
	}
	if got.NOutputsMaxPerSeq != 1 {
		t.Errorf("NOutputsMaxPerSeq = %d, want 1", got.NOutputsMaxPerSeq)
	}
	if got.NCtx != target.NCtx || got.NBatch != target.NBatch || got.NUbatch != target.NUbatch || got.NSeqMax != target.NSeqMax {
		t.Errorf("context sizing = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
			got.NCtx, got.NBatch, got.NUbatch, got.NSeqMax,
			target.NCtx, target.NBatch, target.NUbatch, target.NSeqMax)
	}
	if got.RopeScalingType != target.RopeScalingType || got.RopeFreqBase != target.RopeFreqBase || got.RopeFreqScale != target.RopeFreqScale {
		t.Errorf("RoPE params = (%d, %g, %g), want (%d, %g, %g)",
			got.RopeScalingType, got.RopeFreqBase, got.RopeFreqScale,
			target.RopeScalingType, target.RopeFreqBase, target.RopeFreqScale)
	}
	if got.YarnExtFactor != target.YarnExtFactor || got.YarnAttnFactor != target.YarnAttnFactor || got.YarnBetaFast != target.YarnBetaFast || got.YarnBetaSlow != target.YarnBetaSlow || got.YarnOrigCtx != target.YarnOrigCtx {
		t.Errorf("YaRN params = (%g, %g, %g, %g, %d), want (%g, %g, %g, %g, %d)",
			got.YarnExtFactor, got.YarnAttnFactor, got.YarnBetaFast, got.YarnBetaSlow, got.YarnOrigCtx,
			target.YarnExtFactor, target.YarnAttnFactor, target.YarnBetaFast, target.YarnBetaSlow, target.YarnOrigCtx)
	}
}
