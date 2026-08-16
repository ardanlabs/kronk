package speculation

import "testing"

func TestResolve(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		want    Source
		wantErr bool
	}{
		{"auto disabled without capability", Config{Mode: ModeAuto}, SourceNone, false},
		{"disabled ignores all capabilities", Config{Mode: ModeDisabled, ClassicConfigured: true, EmbeddedMTP: true, MTPAvailable: true}, SourceNone, false},
		{"auto prefers classic", Config{Mode: ModeAuto, ClassicConfigured: true, ClassicNDraft: 5, CompanionMTP: true, EmbeddedMTP: true, MTPAvailable: true}, SourceClassic, false},
		{"auto prefers companion MTP", Config{Mode: ModeAuto, CompanionMTP: true, EmbeddedMTP: true, MTPNDraft: 3, MTPAvailable: true}, SourceMTPCompanion, false},
		{"auto selects embedded MTP", Config{Mode: ModeAuto, EmbeddedMTP: true, MTPNDraft: 3, MTPAvailable: true}, SourceMTPEmbedded, false},
		{"classic requires model", Config{Mode: ModeClassic}, SourceNone, true},
		{"MTP rejects classic model", Config{Mode: ModeMTP, ClassicConfigured: true}, SourceNone, true},
		{"MTP requires source", Config{Mode: ModeMTP, MTPAvailable: true}, SourceNone, true},
		{"MTP requires library support", Config{Mode: ModeMTP, EmbeddedMTP: true}, SourceNone, true},
		{"unknown mode rejected", Config{Mode: "future"}, SourceNone, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Resolve() error = %v, wantErr %t", err, tt.wantErr)
			}
			if got.Source != tt.want {
				t.Errorf("Source = %d, want %d", got.Source, tt.want)
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
