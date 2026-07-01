package launch

import (
	"os"
	"path/filepath"
	"testing"

	yaml "go.yaml.in/yaml/v2"
)

func TestBuildHermesConfigFromEmpty(t *testing.T) {
	config := buildHermesConfig(nil, "Qwen3-8B-Q8_0", "http://localhost:9999/v1", "kronk", 40960)

	model := hermesStringMap(config["model"])

	if got := model["provider"]; got != "custom" {
		t.Errorf("provider: got %v, want custom", got)
	}
	if got := model["default"]; got != "Qwen3-8B-Q8_0" {
		t.Errorf("default: got %v, want Qwen3-8B-Q8_0", got)
	}
	if got := model["base_url"]; got != "http://localhost:9999/v1" {
		t.Errorf("base_url: got %v, want http://localhost:9999/v1", got)
	}
	if got := model["api_key"]; got != "kronk" {
		t.Errorf("api_key: got %v, want kronk", got)
	}
	if got := model["context_length"]; got != 40960 {
		t.Errorf("context_length: got %v, want 40960", got)
	}
}

func TestBuildHermesConfigOmitsUnknownContext(t *testing.T) {
	config := buildHermesConfig(nil, "m", "http://x/v1", "kronk", 0)

	model := hermesStringMap(config["model"])
	if _, ok := model["context_length"]; ok {
		t.Errorf("context_length should be absent when unknown")
	}
}

func TestBuildHermesConfigPreservesUserData(t *testing.T) {
	// Mimic what YAML v2 produces: nested maps are map[any]any.
	existing := map[string]any{
		"model": map[any]any{
			"provider":   "anthropic",
			"default":    "claude",
			"max_tokens": 8192, // a user model setting we don't manage - must survive.
		},
		"toolsets": []any{"terminal", "web"}, // an unrelated section - must survive.
	}

	config := buildHermesConfig(existing, "new/model", "http://new:2/v1", "${KRONK_TOKEN}", 8192)

	model := hermesStringMap(config["model"])

	// Managed keys are updated.
	if got := model["provider"]; got != "custom" {
		t.Errorf("provider: got %v, want custom", got)
	}
	if got := model["default"]; got != "new/model" {
		t.Errorf("default: got %v, want new/model", got)
	}
	if got := model["api_key"]; got != "${KRONK_TOKEN}" {
		t.Errorf("api_key: got %v, want ${KRONK_TOKEN}", got)
	}

	// Unmanaged model setting preserved.
	if got := model["max_tokens"]; got != 8192 {
		t.Errorf("max_tokens should be preserved, got %v", got)
	}

	// Unrelated section preserved.
	if _, ok := config["toolsets"]; !ok {
		t.Errorf("unrelated toolsets section should be preserved")
	}
}

func TestWriteHermesConfigBacksUpAndIsValidYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERMES_HOME", "")

	path := filepath.Join(home, ".hermes", "config.yaml")

	// First write: no prior file, so no backup.
	if err := writeHermesConfig("a/one", []Model{{ID: "a/one", Name: "a/one", Context: 65536}}); err != nil {
		t.Fatalf("first writeHermesConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config.yaml not written: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup expected on first write")
	}

	// Second write: the prior file should be backed up.
	if err := writeHermesConfig("b/two", []Model{{ID: "b/two", Name: "b/two"}}); err != nil {
		t.Fatalf("second writeHermesConfig: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("backup expected on second write: %v", err)
	}

	// The written file is valid YAML with the custom Kronk provider.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("config.yaml is not valid YAML: %v", err)
	}
	model := hermesStringMap(doc["model"])
	if model["provider"] != "custom" {
		t.Errorf("config.yaml missing custom provider, got %v", model["provider"])
	}
}

func TestWriteHermesConfigRequiresModels(t *testing.T) {
	if err := writeHermesConfig("", nil); err == nil {
		t.Errorf("expected error when no models provided")
	}
}

func TestHermesInstallerCommand(t *testing.T) {
	tests := []struct {
		goos    string
		wantBin string
		wantErr bool
	}{
		{goos: "windows", wantBin: "powershell.exe"},
		{goos: "darwin", wantBin: "bash"},
		{goos: "linux", wantBin: "bash"},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, err := hermesInstallerCommand(tt.goos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tt.goos)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bin != tt.wantBin {
				t.Errorf("bin: got %q, want %q", bin, tt.wantBin)
			}
			if len(args) == 0 {
				t.Errorf("expected non-empty args for %s", tt.goos)
			}
		})
	}
}
