package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildOpenClawConfigFromEmpty(t *testing.T) {
	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true, Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	config := buildOpenClawConfig(nil, chatModels, "Qwen3-8B-Q8_0", "http://localhost:9999/v1", "kronk")

	models := config["models"].(map[string]any)
	providers := models["providers"].(map[string]any)
	provider := providers["kronk"].(map[string]any)

	if got := provider["baseUrl"]; got != "http://localhost:9999/v1" {
		t.Errorf("baseUrl: got %v, want http://localhost:9999/v1", got)
	}
	if got := provider["api"]; got != "openai-completions" {
		t.Errorf("api: got %v, want openai-completions", got)
	}
	if got := provider["apiKey"]; got != "kronk" {
		t.Errorf("apiKey: got %v, want kronk", got)
	}

	entries := provider["models"].([]any)
	if len(entries) != 2 {
		t.Fatalf("expected 2 model entries, got %d", len(entries))
	}

	byID := map[string]map[string]any{}
	for _, m := range entries {
		mo := m.(map[string]any)
		byID[mo["id"].(string)] = mo
	}

	reasoner := byID["Qwen3-8B-Q8_0"]
	if reasoner["reasoning"] != true {
		t.Errorf("reasoning model should have reasoning=true")
	}
	if got := reasoner["contextWindow"]; got != 40960 {
		t.Errorf("contextWindow: got %v, want 40960", got)
	}
	if in := reasoner["input"].([]any); len(in) != 1 {
		t.Errorf("text model input: got %v, want [text]", in)
	}

	vision := byID["Qwen2-VL-7B"]
	if in := vision["input"].([]any); len(in) != 2 {
		t.Errorf("vision model input: got %v, want [text image]", in)
	}
	if _, ok := vision["contextWindow"]; ok {
		t.Errorf("contextWindow should be absent when unknown")
	}

	// Default model primary and allowlist entries are written.
	defaults := config["agents"].(map[string]any)["defaults"].(map[string]any)
	if got := defaults["model"].(map[string]any)["primary"]; got != "kronk/Qwen3-8B-Q8_0" {
		t.Errorf("primary: got %v, want kronk/Qwen3-8B-Q8_0", got)
	}

	allow := defaults["models"].(map[string]any)
	if _, ok := allow["kronk/Qwen3-8B-Q8_0"]; !ok {
		t.Errorf("allowlist missing kronk/Qwen3-8B-Q8_0")
	}
	if _, ok := allow["kronk/Qwen2-VL-7B"]; !ok {
		t.Errorf("allowlist missing kronk/Qwen2-VL-7B")
	}
}

func TestBuildOpenClawConfigPreservesUserData(t *testing.T) {
	existing := map[string]any{
		"models": map[string]any{
			"providers": map[string]any{
				// A provider the user configured themselves - must be untouched.
				"anthropic": map[string]any{
					"baseUrl": "https://api.anthropic.com",
				},
				"kronk": map[string]any{
					"baseUrl": "http://old:1/v1",
					"apiKey":  "old",
					"api":     "openai-completions",
					"models":  []any{map[string]any{"id": "old/managed"}},
				},
			},
		},
		"agents": map[string]any{
			"defaults": map[string]any{
				"model": map[string]any{"primary": "anthropic/claude"},
				"models": map[string]any{
					// A user allowlist entry for another provider - must be preserved.
					"anthropic/claude": map[string]any{},
					// A stale kronk entry - must be dropped.
					"kronk/old": map[string]any{},
				},
			},
		},
	}

	chatModels := []Model{{ID: "new/model", Name: "new/model", Context: 8192}}

	config := buildOpenClawConfig(existing, chatModels, "new/model", "http://new:2/v1", "${KRONK_TOKEN}")

	providers := config["models"].(map[string]any)["providers"].(map[string]any)

	// Other providers untouched.
	if _, ok := providers["anthropic"]; !ok {
		t.Errorf("user's anthropic provider should be preserved")
	}

	provider := providers["kronk"].(map[string]any)
	if got := provider["baseUrl"]; got != "http://new:2/v1" {
		t.Errorf("baseUrl: got %v, want http://new:2/v1", got)
	}
	if got := provider["apiKey"]; got != "${KRONK_TOKEN}" {
		t.Errorf("apiKey: got %v, want ${KRONK_TOKEN}", got)
	}

	// The kronk provider is rebuilt: only the current model remains.
	entries := provider["models"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["id"] != "new/model" {
		t.Errorf("kronk provider should be rebuilt with only the current model, got %v", entries)
	}

	defaults := config["agents"].(map[string]any)["defaults"].(map[string]any)
	if got := defaults["model"].(map[string]any)["primary"]; got != "kronk/new/model" {
		t.Errorf("primary: got %v, want kronk/new/model", got)
	}

	allow := defaults["models"].(map[string]any)
	if _, ok := allow["anthropic/claude"]; !ok {
		t.Errorf("user allowlist entry should be preserved")
	}
	if _, ok := allow["kronk/old"]; ok {
		t.Errorf("stale kronk allowlist entry should be dropped")
	}
	if _, ok := allow["kronk/new/model"]; !ok {
		t.Errorf("current model allowlist entry should be added")
	}
}

func TestWriteOpenClawConfigBacksUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".openclaw", "openclaw.json")

	// First write: no prior file, so no backup.
	if err := writeOpenClawConfig("a/one", []Model{{ID: "a/one", Name: "a/one"}}); err != nil {
		t.Fatalf("first writeOpenClawConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("openclaw.json not written: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup expected on first write")
	}

	// Second write: the prior file should be backed up.
	if err := writeOpenClawConfig("b/two", []Model{{ID: "b/two", Name: "b/two"}}); err != nil {
		t.Fatalf("second writeOpenClawConfig: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("backup expected on second write: %v", err)
	}

	// The written file is valid JSON with our provider.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading openclaw.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("openclaw.json is not valid JSON: %v", err)
	}
	providers, ok := doc["models"].(map[string]any)["providers"].(map[string]any)
	if !ok || providers["kronk"] == nil {
		t.Errorf("openclaw.json missing kronk provider")
	}
}

func TestWriteOpenClawConfigRequiresModels(t *testing.T) {
	if err := writeOpenClawConfig("", nil); err == nil {
		t.Errorf("expected error when no models provided")
	}
}

func TestOpenClawInstallerCommand(t *testing.T) {
	tests := []struct {
		goos    string
		wantErr bool
	}{
		{goos: "windows"},
		{goos: "darwin"},
		{goos: "linux"},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, err := openclawInstallerCommand(tt.goos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s, got nil", tt.goos)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bin != "npm" {
				t.Errorf("bin: got %q, want npm", bin)
			}
			if len(args) == 0 {
				t.Errorf("expected non-empty args for %s", tt.goos)
			}
		})
	}
}
