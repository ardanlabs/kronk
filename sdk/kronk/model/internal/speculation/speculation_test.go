package speculation

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name             string
		cfg              Config
		wantSource       Source
		wantArchitecture MTPArchitecture
		wantArtifact     MTPArtifact
		wantLoadMTP      bool
		wantErr          bool
	}{
		{"auto disabled without capability", Config{Mode: ModeAuto}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, false},
		{"disabled ignores all capabilities", Config{Mode: ModeDisabled, ClassicConfigured: true, EmbeddedMTP: true, MTPAvailable: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, false},
		{"auto prefers classic", Config{Mode: ModeAuto, ClassicConfigured: true, ClassicNDraft: 5, CompanionMTP: true, EmbeddedMTP: true, MTPAvailable: true}, SourceClassic, MTPArchitectureNone, MTPArtifactNone, false, false},
		{"auto prefers companion MTP", Config{Mode: ModeAuto, CompanionMTP: true, EmbeddedMTP: true, MTPNDraft: 3, MTPAvailable: true}, SourceMTP, MTPArchitectureGemmaSharedKV, MTPArtifactCompanion, false, false},
		{"auto selects own-KV companion MTP", Config{Mode: ModeAuto, OwnKVCompanionMTP: true, EmbeddedMTP: true, MTPNDraft: 3, MTPAvailable: true}, SourceMTP, MTPArchitectureQwen35OwnKV, MTPArtifactCompanion, false, false},
		{"auto selects embedded MTP", Config{Mode: ModeAuto, EmbeddedMTP: true, MTPNDraft: 3, MTPAvailable: true}, SourceMTP, MTPArchitectureQwen35OwnKV, MTPArtifactEmbedded, true, false},
		{"classic requires model", Config{Mode: ModeClassic}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP rejects classic model", Config{Mode: ModeMTP, ClassicConfigured: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP requires source", Config{Mode: ModeMTP, MTPAvailable: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP requires library support", Config{Mode: ModeMTP, EmbeddedMTP: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"unknown mode rejected", Config{Mode: "future"}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got.Source != tt.wantSource || got.MTPArchitecture != tt.wantArchitecture || got.MTPArtifact != tt.wantArtifact {
				t.Errorf("plan = %+v, want source %d architecture %d artifact %d", got, tt.wantSource, tt.wantArchitecture, tt.wantArtifact)
			}
			if got.LoadMTP != tt.wantLoadMTP {
				t.Errorf("LoadMTP = %t, want %t", got.LoadMTP, tt.wantLoadMTP)
			}
		})
	}
}

func TestPlanRowsPerSequence(t *testing.T) {
	if got := (Plan{}).RowsPerSequence(); got != 1 {
		t.Errorf("disabled rows = %d, want 1", got)
	}
	if got := (Plan{Source: SourceClassic, NDraft: 5, Available: true}).RowsPerSequence(); got != 6 {
		t.Errorf("classic rows = %d, want 6", got)
	}
}
