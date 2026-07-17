package models

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestResolveAdapters(t *testing.T) {
	basePath := t.TempDir()
	m := Models{basePath: basePath}

	idPath := filepath.Join(basePath, "lora", "acme", "support.gguf")
	if err := os.MkdirAll(filepath.Dir(idPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(idPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	plainIDPath := filepath.Join(basePath, "lora", "support.gguf")
	if err := os.WriteFile(plainIDPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	externalPath := filepath.Join(t.TempDir(), "external.gguf")
	if err := os.WriteFile(externalPath, []byte("adapter"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	directoryPath := filepath.Join(basePath, "directory.gguf")
	if err := os.Mkdir(directoryPath, 0755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	zero := float32(0)
	tests := []struct {
		name      string
		adapters  []AdapterConfig
		wantPath  string
		wantScale float32
		wantErr   string
	}{
		{
			name:      "id resolves under lora folder with default scale",
			adapters:  []AdapterConfig{{ID: "acme/support"}},
			wantPath:  idPath,
			wantScale: 1,
		},
		{
			name:      "id without organization resolves under lora folder",
			adapters:  []AdapterConfig{{ID: "support"}},
			wantPath:  plainIDPath,
			wantScale: 1,
		},
		{
			name:      "absolute path preserves explicit zero scale",
			adapters:  []AdapterConfig{{Path: externalPath, PtrScale: &zero}},
			wantPath:  externalPath,
			wantScale: 0,
		},
		{name: "id and path are mutually exclusive", adapters: []AdapterConfig{{ID: "acme/support", Path: externalPath}}, wantErr: "exactly one"},
		{name: "id is required when path is absent", adapters: []AdapterConfig{{}}, wantErr: "exactly one"},
		{name: "id rejects traversal", adapters: []AdapterConfig{{ID: "../support"}}, wantErr: "invalid path component"},
		{name: "id rejects extension", adapters: []AdapterConfig{{ID: "acme/support.gguf"}}, wantErr: "omit the .gguf extension"},
		{name: "path must be absolute", adapters: []AdapterConfig{{Path: "support.gguf"}}, wantErr: "must be absolute"},
		{name: "adapter must exist", adapters: []AdapterConfig{{Path: filepath.Join(basePath, "missing.gguf")}}, wantErr: "no such file"},
		{name: "adapter must be a regular file", adapters: []AdapterConfig{{Path: directoryPath}}, wantErr: "not a regular file"},
		{name: "scale must be finite", adapters: []AdapterConfig{{Path: externalPath, PtrScale: new(float32(math.NaN()))}}, wantErr: "must be finite"},
		{name: "duplicate paths are rejected", adapters: []AdapterConfig{{Path: externalPath}, {Path: externalPath}}, wantErr: "duplicate adapter path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := m.resolveAdapters(tt.adapters)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveAdapters() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAdapters() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("resolveAdapters() returned %d adapters, want 1", len(got))
			}
			if got[0].Path != tt.wantPath {
				t.Errorf("Path = %q, want %q", got[0].Path, tt.wantPath)
			}
			if got[0].Scale != tt.wantScale {
				t.Errorf("Scale = %g, want %g", got[0].Scale, tt.wantScale)
			}
		})
	}
}

func TestAdapterConfigYAML(t *testing.T) {
	data := []byte(`test-model:
  adapters:
    - id: acme/support
    - path: /opt/adapters/legal.gguf
      scale: 0
`)

	var configs map[string]ModelConfig
	if err := yaml.Unmarshal(data, &configs); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	adapters := configs["test-model"].Adapters
	if len(adapters) != 2 {
		t.Fatalf("Adapters length = %d, want 2", len(adapters))
	}
	if adapters[0].ID != "acme/support" {
		t.Errorf("Adapters[0].ID = %q, want %q", adapters[0].ID, "acme/support")
	}
	if adapters[0].PtrScale != nil {
		t.Errorf("Adapters[0].PtrScale = %v, want nil", adapters[0].PtrScale)
	}
	if adapters[1].Path != "/opt/adapters/legal.gguf" {
		t.Errorf("Adapters[1].Path = %q, want %q", adapters[1].Path, "/opt/adapters/legal.gguf")
	}
	if adapters[1].PtrScale == nil || *adapters[1].PtrScale != 0 {
		t.Errorf("Adapters[1].PtrScale = %v, want pointer to 0", adapters[1].PtrScale)
	}
}

func TestMergeModelConfigAdapters(t *testing.T) {
	dst := ModelConfig{Adapters: []AdapterConfig{{ID: "old"}}}
	src := ModelConfig{Adapters: []AdapterConfig{{ID: "new"}}}

	MergeModelConfig(&dst, src)

	if len(dst.Adapters) != 1 || dst.Adapters[0].ID != "new" {
		t.Errorf("Adapters = %+v, want replacement adapter", dst.Adapters)
	}
}
