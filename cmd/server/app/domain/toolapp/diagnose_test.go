package toolapp

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestSetDiagnoseEnv(t *testing.T) {
	tests := []struct {
		name  string
		env   []string
		key   string
		value string
		want  []string
	}{
		{
			name:  "replace existing value",
			env:   []string{"A=1", "KRONK_PROCESSOR=cuda"},
			key:   "KRONK_PROCESSOR",
			value: "vulkan",
			want:  []string{"A=1", "KRONK_PROCESSOR=vulkan"},
		},
		{
			name:  "append missing value",
			env:   []string{"A=1"},
			key:   "KRONK_PROCESSOR",
			value: "vulkan",
			want:  []string{"A=1", "KRONK_PROCESSOR=vulkan"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := setDiagnoseEnv(tt.env, tt.key, tt.value)
			if !slices.Equal(got, tt.want) {
				t.Errorf("environment: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCanonicalRuntimeBasePath(t *testing.T) {
	canonicalRoot := filepath.Join("srv", "kronk", "libraries")
	explicitRoot := filepath.Join("opt", "custom", "cuda")
	tests := []struct {
		name      string
		root      string
		libPath   string
		want      string
		canonical bool
	}{
		{
			name:      "canonical install",
			root:      canonicalRoot,
			libPath:   filepath.Join(canonicalRoot, "linux", "amd64", "vulkan"),
			want:      filepath.Dir(canonicalRoot),
			canonical: true,
		},
		{
			name:    "explicit bundle path",
			root:    explicitRoot,
			libPath: explicitRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := canonicalRuntimeBasePath(tt.root, tt.libPath, "linux", "amd64", "vulkan")
			if ok != tt.canonical {
				t.Errorf("canonical: got %v, want %v", ok, tt.canonical)
			}
			if got != tt.want {
				t.Errorf("base path: got %q, want %q", got, tt.want)
			}
		})
	}
}
