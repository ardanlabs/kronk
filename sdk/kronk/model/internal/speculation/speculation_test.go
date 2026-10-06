package speculation

import (
	"strings"
	"testing"
)

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
		{"disabled ignores all capabilities", Config{Mode: ModeDisabled, ClassicConfigured: true, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, false},
		{"auto prefers classic", Config{Mode: ModeAuto, ClassicConfigured: true, ClassicNDraft: 5, CompanionMTPArchitecture: MTPArchitectureGemmaSharedKV, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceClassic, MTPArchitectureNone, MTPArtifactNone, false, false},
		{"auto prefers companion MTP", Config{Mode: ModeAuto, CompanionMTPArchitecture: MTPArchitectureGemmaSharedKV, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureGemmaSharedKV, MTPArtifactCompanion, false, false},
		{"auto selects own-KV companion MTP", Config{Mode: ModeAuto, CompanionMTPArchitecture: MTPArchitectureQwen35OwnKV, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureQwen35OwnKV, MTPArtifactCompanion, false, false},
		{"auto selects embedded MTP", Config{Mode: ModeAuto, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureQwen35OwnKV, MTPArtifactEmbedded, true, false},
		{"auto does not load unavailable embedded MTP", Config{Mode: ModeAuto, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureQwen35OwnKV, MTPArtifactEmbedded, false, false},
		{"auto selects embedded qwen4exp MTP", Config{Mode: ModeAuto, EmbeddedMTPArchitecture: MTPArchitectureQwen4ExpOwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureQwen4ExpOwnKV, MTPArtifactEmbedded, true, false},
		{"explicit qwen4exp selects companion MTP", Config{Mode: ModeMTP, CompanionMTPArchitecture: MTPArchitectureQwen4ExpOwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureQwen4ExpOwnKV, MTPArtifactCompanion, false, false},
		{"auto selects embedded Nemotron MTP", Config{Mode: ModeAuto, EmbeddedMTPArchitecture: MTPArchitectureNemotronOwnKV, MTPNDraft: 3, MTPAvailable: true, MTPArchitectureEnabled: true}, SourceMTP, MTPArchitectureNemotronOwnKV, MTPArtifactEmbedded, true, false},
		{"classic requires model", Config{Mode: ModeClassic}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP rejects classic model", Config{Mode: ModeMTP, ClassicConfigured: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP requires source", Config{Mode: ModeMTP, MTPAvailable: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
		{"MTP requires library support", Config{Mode: ModeMTP, EmbeddedMTPArchitecture: MTPArchitectureQwen35OwnKV, MTPArchitectureEnabled: true}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
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

func TestResolveGLM5Next(t *testing.T) {
	cfg := Config{
		Mode:                    ModeAuto,
		EmbeddedMTPArchitecture: MTPArchitectureGLM5Next,
		MTPAvailable:            true,
	}

	plan, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve(auto) error = %v", err)
	}
	if plan.Source != SourceNone || plan.LoadMTP {
		t.Fatalf("Resolve(auto) = %+v, want target-only plan", plan)
	}

	cfg.Mode = ModeMTP
	_, err = Resolve(cfg)
	if err == nil || !strings.Contains(err.Error(), "MTP architecture glm5-next is unsupported") {
		t.Fatalf("Resolve(mtp) error = %v, want unsupported glm5-next error", err)
	}
}
