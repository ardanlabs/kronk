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
		{"unknown mode rejected", Config{Mode: Mode{value: "future"}}, SourceNone, MTPArchitectureNone, MTPArtifactNone, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got.Source != tt.wantSource || got.MTPArchitecture != tt.wantArchitecture || got.MTPArtifact != tt.wantArtifact {
				t.Errorf("plan = %+v, want source %s architecture %s artifact %s", got, tt.wantSource, tt.wantArchitecture, tt.wantArtifact)
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

func TestResolveDSpark(t *testing.T) {
	for _, tt := range []struct {
		name                          string
		mode                          Mode
		classic, companion, available bool
		want                          Source
		wantErr                       string
	}{
		{"auto prefers DSpark over MTP", ModeAuto, false, true, true, SourceDSpark, ""},
		{"auto stages unavailable DSpark without activation", ModeAuto, false, true, false, SourceDSpark, ""},
		{"auto classic still wins", ModeAuto, true, true, true, SourceClassic, ""},
		{"disabled ignores all", ModeDisabled, true, true, true, SourceNone, ""},
		{"explicit MTP ignores DSpark", ModeMTP, false, true, true, SourceMTP, ""},
		{"explicit classic ignores DSpark", ModeClassic, true, true, true, SourceClassic, ""},
		{"explicit DSpark", ModeDSpark, false, true, true, SourceDSpark, ""},
		{"explicit DSpark missing", ModeDSpark, false, false, true, SourceNone, "requires a compatible DSpark companion in DrafterFile"},
		{"explicit DSpark unavailable", ModeDSpark, false, true, false, SourceNone, "DSpark execution is not yet available"},
		{"explicit DSpark conflicts with classic", ModeDSpark, true, true, true, SourceNone, "cannot use a separate draft model"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := Resolve(Config{
				Mode: tt.mode, ClassicConfigured: tt.classic, ClassicNDraft: 5,
				DSparkConfigured: tt.companion, DSparkAvailable: tt.available, DSparkNDraft: 7,
				CompanionMTPArchitecture: MTPArchitectureGemmaSharedKV,
				EmbeddedMTPArchitecture:  MTPArchitectureQwen35OwnKV,
				MTPAvailable:             true, MTPArchitectureEnabled: true, MTPNDraft: 3,
			})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Resolve() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || plan.Source != tt.want {
				t.Fatalf("Resolve() = %+v, %v, want source %s", plan, err, tt.want)
			}
			if plan.Source == SourceDSpark {
				if plan.Active() != tt.available || plan.NDraft != 7 || plan.MTP() || plan.LoadMTP {
					t.Errorf("DSpark plan = %+v, active = %t", plan, plan.Active())
				}
				wantRows := 1
				if tt.available {
					wantRows = 8
				}
				if plan.RowsPerSequence() != wantRows {
					t.Errorf("rows = %d, want %d", plan.RowsPerSequence(), wantRows)
				}
			}
		})
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
