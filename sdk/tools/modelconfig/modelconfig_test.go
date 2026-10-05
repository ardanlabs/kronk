package modelconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		yaml       string
		wantModel  string
		wantBucky  int
		wantMalina int
		wantErr    bool
	}{
		{
			name:      "version one remains compatible",
			yaml:      "version: 1\nmodels:\n  owner/model:\n    nseq-max: 2\n",
			wantModel: "owner/model",
		},
		{
			name:       "version two backend settings",
			yaml:       "version: 2\nbucky-models:\n  tiny:\n    nseq-max: 2\n    admission-timeout: 30s\nmalina-models:\n  sd-1.5:\n    concurrency: 3\n    admission-timeout: 2m\n",
			wantBucky:  2,
			wantMalina: 3,
		},
		{
			name:      "version one accepts known backend settings",
			yaml:      "version: 1\nbucky-models:\n  tiny:\n    nseq-max: 2\n    admission-timeout: 30s\n",
			wantBucky: 2,
		},
		{
			name:    "invalid bucky concurrency",
			yaml:    "version: 2\nbucky-models:\n  tiny:\n    nseq-max: 0\n",
			wantErr: true,
		},
		{
			name:    "invalid malina queue",
			yaml:    "version: 2\nmalina-models:\n  sd-1.5:\n    queue-depth: -1\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model_config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			doc, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("Load: got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if tt.wantModel != "" {
				if _, exists := doc.Models[tt.wantModel]; !exists {
					t.Errorf("Models: got keys %v, want %q", doc.Models, tt.wantModel)
				}
			}
			if tt.wantBucky > 0 {
				cfg := doc.BuckyModels["tiny"]
				if cfg.NSeqMax == nil || *cfg.NSeqMax != tt.wantBucky {
					t.Errorf("Bucky NSeqMax: got %v, want %d", cfg.NSeqMax, tt.wantBucky)
				}
				if cfg.AdmissionTimeout == nil || time.Duration(*cfg.AdmissionTimeout) != 30*time.Second {
					t.Errorf("Bucky AdmissionTimeout: got %v, want %s", cfg.AdmissionTimeout, 30*time.Second)
				}
			}
			if tt.wantMalina > 0 {
				cfg := doc.MalinaModels["sd-1.5"]
				if cfg.Concurrency == nil || *cfg.Concurrency != tt.wantMalina {
					t.Errorf("Malina Concurrency: got %v, want %d", cfg.Concurrency, tt.wantMalina)
				}
			}
		})
	}
}
