package models

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"go.yaml.in/yaml/v2"
)

func Test_fullPathLookupKeys(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    []string
	}{
		{
			name:    "bare model name",
			modelID: "Qwopus3.5-4B-Coder.Q8_0",
			want:    nil,
		},
		{
			name:    "provider/model",
			modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
			want:    []string{"mradermacher/Qwopus3.5-4B-Coder.Q8_0"},
		},
		{
			name:    "provider/model/profile resolves base model",
			modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT",
			want:    []string{"mradermacher/Qwopus3.5-4B-Coder.Q8_0"},
		},
		{name: "too many segments", modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0/playground/sess-1", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fullPathLookupKeys(tt.modelID)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilesPreservesPublicFileContract(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	modelFile := filepath.Join(m.Path(), "provider", "family", "model.gguf")
	if err := os.MkdirAll(filepath.Dir(modelFile), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(modelFile, []byte("model"), 0644); err != nil {
		t.Fatalf("WriteFile model: %v", err)
	}

	index, err := yaml.Marshal(map[string]Path{
		"provider/model": {
			ModelFiles: []string{modelFile},
			Validated:  true,
		},
	})
	if err != nil {
		t.Fatalf("Marshal index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(m.Path(), indexFile), index, 0644); err != nil {
		t.Fatalf("WriteFile index: %v", err)
	}

	files, err := m.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Files length: got %d, want 1", len(files))
	}
	if files[0].ID != "model" {
		t.Errorf("ID: got %q, want %q", files[0].ID, "model")
	}
	if files[0].OwnedBy != "provider" {
		t.Errorf("OwnedBy: got %q, want %q", files[0].OwnedBy, "provider")
	}
	if !files[0].Validated {
		t.Error("Validated: got false, want true")
	}

	if _, ok := m.LookupFile("provider/model"); !ok {
		t.Error("LookupFile canonical model: got false, want true")
	}
	if _, ok := m.LookupFile("provider/model/AGENT"); !ok {
		t.Error("LookupFile profile: got false, want true")
	}
	if _, ok := m.LookupFile("model"); ok {
		t.Error("LookupFile bare model: got true, want false")
	}

	downloaded, validated := m.IndexState()
	if !downloaded["provider/model"] {
		t.Error("IndexState downloaded: got false, want true")
	}
	if !validated["provider/model"] {
		t.Error("IndexState validated: got false, want true")
	}

	if _, err := m.FullPath("model"); !errors.Is(err, ErrInvalidModelID) {
		t.Errorf("FullPath bare model: got %v, want ErrInvalidModelID", err)
	}
}
