package models

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadModelConfig(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantID  string
		wantErr bool
	}{
		{
			name:   "version zero",
			yaml:   "owner/model:\n  context-window: 4096\n",
			wantID: "owner/model",
		},
		{
			name:   "version one",
			yaml:   "version: 1\nmodels:\n  owner/model:\n    context-window: 8192\nkms:\n  web:\n    api-host: localhost:9000\n",
			wantID: "owner/model",
		},
		{
			name:    "unsupported version",
			yaml:    "version: 2\nmodels: {}\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "model_config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}

			configs, err := LoadModelConfig(path)
			if tt.wantErr {
				if err == nil {
					t.Fatal("LoadModelConfig: got nil error, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadModelConfig: %v", err)
			}
			if _, exists := configs[tt.wantID]; !exists {
				t.Errorf("LoadModelConfig: got keys %v, want %q", configs, tt.wantID)
			}
		})
	}
}
