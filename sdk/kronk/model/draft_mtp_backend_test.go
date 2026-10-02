package model

import "testing"

func TestMTPBackendForPlan(t *testing.T) {
	tests := []struct {
		name              string
		plan              speculationPlan
		wantName          string
		wantArtifact      mtpArtifact
		wantSharedKV      bool
		wantFixedPosition bool
		wantWideEmbedding bool
		wantErr           bool
	}{
		{"embedded Qwen35", speculationPlan{MTPArchitecture: mtpArchitectureQwen35OwnKV, MTPArtifact: mtpArtifactEmbedded}, "qwen35-own-kv", mtpArtifactEmbedded, false, false, false, false},
		{"companion Qwen35", speculationPlan{MTPArchitecture: mtpArchitectureQwen35OwnKV, MTPArtifact: mtpArtifactCompanion}, "qwen35-own-kv", mtpArtifactCompanion, false, false, false, false},
		{"embedded Qwen4Exp scaffold", speculationPlan{MTPArchitecture: mtpArchitectureQwen4ExpOwnKV, MTPArtifact: mtpArtifactEmbedded}, "qwen4exp-own-kv", mtpArtifactEmbedded, false, false, true, false},
		{"companion Qwen4Exp scaffold", speculationPlan{MTPArchitecture: mtpArchitectureQwen4ExpOwnKV, MTPArtifact: mtpArtifactCompanion}, "qwen4exp-own-kv", mtpArtifactCompanion, false, false, true, false},
		{"companion Gemma", speculationPlan{MTPArchitecture: mtpArchitectureGemmaSharedKV, MTPArtifact: mtpArtifactCompanion}, "gemma-shared-kv", 0, true, true, false, false},
		{"embedded Gemma rejected", speculationPlan{MTPArchitecture: mtpArchitectureGemmaSharedKV, MTPArtifact: mtpArtifactEmbedded}, "", 0, false, false, false, true},
		{"missing architecture rejected", speculationPlan{MTPArtifact: mtpArtifactCompanion}, "", 0, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend, err := mtpBackendForPlan(tt.plan)
			if (err != nil) != tt.wantErr {
				t.Fatalf("mtpBackendForPlan() error = %v, wantErr %t", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got := backend.name(); got != tt.wantName {
				t.Errorf("name = %q, want %q", got, tt.wantName)
			}
			if qwen, ok := backend.(ownKVMTPBackend); ok {
				if qwen.artifact != tt.wantArtifact {
					t.Errorf("artifact = %d, want %d", qwen.artifact, tt.wantArtifact)
				}
				if qwen.wideEmbedding != tt.wantWideEmbedding {
					t.Errorf("wideEmbedding = %t, want %t", qwen.wideEmbedding, tt.wantWideEmbedding)
				}
			}
			if got := backend.sharedKV(); got != tt.wantSharedKV {
				t.Errorf("sharedKV = %t, want %t", got, tt.wantSharedKV)
			}
			if got := backend.fixedDraftPosition(); got != tt.wantFixedPosition {
				t.Errorf("fixedDraftPosition = %t, want %t", got, tt.wantFixedPosition)
			}
		})
	}
}
