package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildVSCodeChatLanguageModelsFromEmpty(t *testing.T) {
	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	out := buildVSCodeChatLanguageModels(nil, chatModels, "http://localhost:9999/v1", "kronk")

	if len(out) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(out))
	}

	prov := out[0]
	if prov["name"] != vscodeProviderName {
		t.Errorf("name: got %v, want %v", prov["name"], vscodeProviderName)
	}
	if prov["vendor"] != vscodeVendor {
		t.Errorf("vendor: got %v, want %v", prov["vendor"], vscodeVendor)
	}
	if prov["apiKey"] != "kronk" {
		t.Errorf("apiKey: got %v, want kronk", prov["apiKey"])
	}
	if prov["apiType"] != "chat-completions" {
		t.Errorf("apiType: got %v, want chat-completions", prov["apiType"])
	}

	models := prov["models"].([]any)
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	byID := map[string]map[string]any{}
	for _, m := range models {
		mo := m.(map[string]any)
		byID[mo["id"].(string)] = mo
	}

	text := byID["Qwen3-8B-Q8_0"]
	if got := text["url"]; got != "http://localhost:9999/v1/chat/completions" {
		t.Errorf("url: got %v", got)
	}
	if text["toolCalling"] != true {
		t.Errorf("toolCalling should be true")
	}
	if _, ok := text["vision"]; ok {
		t.Errorf("non-vision model should not have vision key")
	}
	// Context 40960: reserve 1/4 for output.
	if got := text["maxOutputTokens"]; got != 40960/vscodeOutputReserveFraction {
		t.Errorf("maxOutputTokens: got %v", got)
	}
	if got := text["maxInputTokens"]; got != 40960-40960/vscodeOutputReserveFraction {
		t.Errorf("maxInputTokens: got %v", got)
	}

	vision := byID["Qwen2-VL-7B"]
	if vision["vision"] != true {
		t.Errorf("vision model should have vision=true")
	}
	// Unknown context window → no token budget keys.
	if _, ok := vision["maxOutputTokens"]; ok {
		t.Errorf("maxOutputTokens should be absent when context unknown")
	}
}

func TestBuildVSCodeChatLanguageModelsPreservesOthers(t *testing.T) {
	existing := []map[string]any{
		// A provider the user configured themselves - must be untouched.
		{"name": "OpenAI", "vendor": "openai"},
		// A stale Kronk provider - must be replaced.
		{"name": vscodeProviderName, "vendor": vscodeVendor, "apiKey": "old"},
	}

	chatModels := []Model{{ID: "new/model", Name: "new/model", Context: 8192}}

	out := buildVSCodeChatLanguageModels(existing, chatModels, "http://new:2/v1", "${KRONK_TOKEN}")

	var kronkCount int
	var sawOpenAI bool
	for _, e := range out {
		switch e["name"] {
		case "OpenAI":
			sawOpenAI = true
		case vscodeProviderName:
			kronkCount++
			if e["apiKey"] != "${KRONK_TOKEN}" {
				t.Errorf("apiKey: got %v, want ${KRONK_TOKEN}", e["apiKey"])
			}
		}
	}

	if !sawOpenAI {
		t.Errorf("user's OpenAI provider should be preserved")
	}
	if kronkCount != 1 {
		t.Errorf("expected exactly 1 Kronk provider, got %d", kronkCount)
	}
}

func TestBuildVSCodeSettingsPreservesUserData(t *testing.T) {
	settings := map[string]any{
		"editor.fontSize": float64(14),
		"github.copilot.chat.customOAIModels": map[string]any{
			"user/custom": map[string]any{"name": "user/custom"},
		},
	}

	chatModels := []Model{{ID: "new/model", Name: "new/model", Context: 8192}}

	out := buildVSCodeSettings(settings, chatModels, "http://new:2/v1")

	if out["editor.fontSize"] != float64(14) {
		t.Errorf("unrelated setting should be preserved")
	}

	oai := out["github.copilot.chat.customOAIModels"].(map[string]any)
	if _, ok := oai["user/custom"]; !ok {
		t.Errorf("user-added custom model should be preserved")
	}
	if _, ok := oai["new/model"]; !ok {
		t.Errorf("current model should be added")
	}
}

func TestWriteVSCodeChatModelsBacksUp(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// APPDATA is used on Windows; harmless elsewhere.
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	path, err := vscodeUserPath("chatLanguageModels.json")
	if err != nil {
		t.Fatalf("vscodeUserPath: %v", err)
	}

	models := []Model{{ID: "a/one", Name: "a/one", Context: 8192}}

	// First write: no prior file, so no backup.
	if err := writeVSCodeChatModels(models, "http://localhost:9999/v1", "kronk"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("chatLanguageModels.json not written: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("no backup expected on first write")
	}

	// Second write: the prior file should be backed up.
	if err := writeVSCodeChatModels(models, "http://localhost:9999/v1", "kronk"); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("backup expected on second write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config: %v", err)
	}
	var doc []map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}
}

func TestWriteVSCodeCustomOAIModelsRefusesJSONC(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))

	path, err := vscodeUserPath("settings.json")
	if err != nil {
		t.Fatalf("vscodeUserPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a JSONC file (with a comment) that must not be clobbered.
	jsonc := []byte("{\n  // a comment\n  \"editor.fontSize\": 14\n}\n")
	if err := os.WriteFile(path, jsonc, 0o644); err != nil {
		t.Fatalf("seed settings.json: %v", err)
	}

	err = writeVSCodeCustomOAIModels([]Model{{ID: "a/one", Name: "a/one"}}, "http://localhost:9999/v1")
	if err == nil {
		t.Fatalf("expected error for JSONC settings.json")
	}

	// The original file must be untouched.
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("reading settings.json: %v", readErr)
	}
	if string(got) != string(jsonc) {
		t.Errorf("JSONC settings.json should be left untouched")
	}
}

func TestWriteVSCodeConfigRequiresModels(t *testing.T) {
	if err := writeVSCodeConfig(nil); err == nil {
		t.Errorf("expected error when no models provided")
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.104", "1.104", 0},
		{"1.103", "1.104", -1},
		{"1.105", "1.104", 1},
		{"1.104.2", "1.104", 1},
		{"0.40.9", "0.41.0", -1},
		{"0.41.0", "0.41.0", 0},
		{"2.0", "1.999", 1},
	}

	for _, tt := range tests {
		if got := compareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("compareVersions(%q, %q): got %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}
