package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPiConfigFromEmpty(t *testing.T) {
	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true, Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	config := buildPiConfig(nil, chatModels, "http://localhost:9999/v1", "kronk")

	providers := config["providers"].(map[string]any)
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

	models := provider["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	byID := map[string]map[string]any{}
	for _, m := range models {
		mo := m.(map[string]any)
		byID[mo["id"].(string)] = mo
	}

	reasoner := byID["Qwen3-8B-Q8_0"]
	if reasoner[piLaunchMarker] != true {
		t.Errorf("managed model should carry the launch marker")
	}
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
	// Unknown context window → no contextWindow key.
	if _, ok := vision["contextWindow"]; ok {
		t.Errorf("contextWindow should be absent when unknown")
	}
}

func TestBuildPiConfigPreservesUserData(t *testing.T) {
	existing := map[string]any{
		"providers": map[string]any{
			// A provider the user configured themselves - must be untouched.
			"anthropic": map[string]any{
				"baseUrl": "https://api.anthropic.com",
			},
			"kronk": map[string]any{
				"baseUrl": "http://old:1/v1",
				"api":     "openai-completions",
				"apiKey":  "old",
				"models": []any{
					// A user-added model under our provider - must be preserved.
					map[string]any{"id": "user/custom"},
					// A stale managed model - must be dropped.
					map[string]any{"id": "old/managed", piLaunchMarker: true},
				},
			},
		},
	}

	chatModels := []Model{{ID: "new/model", Name: "new/model", Context: 8192}}

	config := buildPiConfig(existing, chatModels, "http://new:2/v1", "$KRONK_TOKEN")

	providers := config["providers"].(map[string]any)

	// Other providers untouched.
	if _, ok := providers["anthropic"]; !ok {
		t.Errorf("user's anthropic provider should be preserved")
	}

	provider := providers["kronk"].(map[string]any)
	// Provider re-pointed at the current server.
	if got := provider["baseUrl"]; got != "http://new:2/v1" {
		t.Errorf("baseUrl: got %v, want http://new:2/v1", got)
	}
	if got := provider["apiKey"]; got != "$KRONK_TOKEN" {
		t.Errorf("apiKey: got %v, want $KRONK_TOKEN", got)
	}

	models := provider["models"].([]any)
	ids := map[string]bool{}
	for _, m := range models {
		ids[m.(map[string]any)["id"].(string)] = true
	}
	if !ids["user/custom"] {
		t.Errorf("user-added model should be preserved")
	}
	if ids["old/managed"] {
		t.Errorf("stale managed model should be dropped")
	}
	if !ids["new/model"] {
		t.Errorf("current model should be added")
	}
}

func TestHasModelArg(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{"--help"}, false},
		{[]string{"--model", "x"}, true},
		{[]string{"-m", "x"}, true},
		{[]string{"--model=x"}, true},
		{[]string{"-m=x"}, true},
		{[]string{"--foo", "--model", "x"}, true},
	}

	for _, tt := range tests {
		if got := hasModelArg(tt.args); got != tt.want {
			t.Errorf("hasModelArg(%v): got %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestWritePiConfigBacksUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := filepath.Join(home, ".pi", "agent", "models.json")

	// First write: no prior file, so no backup.
	if err := writePiConfig([]Model{{ID: "a/one", Name: "a/one"}}); err != nil {
		t.Fatalf("first writePiConfig: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("models.json not written: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup expected on first write")
	}

	// Second write: the prior file should be backed up.
	if err := writePiConfig([]Model{{ID: "b/two", Name: "b/two"}}); err != nil {
		t.Fatalf("second writePiConfig: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("backup expected on second write: %v", err)
	}

	// The written file is valid JSON with our provider.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading models.json: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("models.json is not valid JSON: %v", err)
	}
	providers, ok := doc["providers"].(map[string]any)
	if !ok || providers["kronk"] == nil {
		t.Errorf("models.json missing kronk provider")
	}
}

func TestWritePiConfigRequiresModels(t *testing.T) {
	if err := writePiConfig(nil); err == nil {
		t.Errorf("expected error when no models provided")
	}
}

func TestPiInstallerCommand(t *testing.T) {
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
			bin, args, err := piInstallerCommand(tt.goos)
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
