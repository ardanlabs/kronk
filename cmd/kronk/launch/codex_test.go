package launch

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// codexArgMap turns the "-c key=value" / "-m model" argument slice into a
// lookup of override key -> raw value and records the -m model, so tests can
// assert on individual settings without depending on argument order.
func codexArgMap(t *testing.T, args []string) (overrides map[string]string, model string) {
	t.Helper()

	overrides = map[string]string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c":
			if i+1 >= len(args) {
				t.Fatalf("-c without a following value in %v", args)
			}
			i++
			key, val, ok := strings.Cut(args[i], "=")
			if !ok {
				t.Fatalf("override %q is not key=value", args[i])
			}
			overrides[key] = val
		case "-m":
			if i+1 >= len(args) {
				t.Fatalf("-m without a following value in %v", args)
			}
			i++
			model = args[i]
		}
	}

	return overrides, model
}

func TestBuildCodexArgs(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")

	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true, Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	args, err := buildCodexArgs("Qwen3-8B-Q8_0", chatModels, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrides, model := codexArgMap(t, args)

	if model != "Qwen3-8B-Q8_0" {
		t.Errorf("model: got %q, want Qwen3-8B-Q8_0", model)
	}
	if got := overrides["model_provider"]; got != `"kronk"` {
		t.Errorf("model_provider: got %q, want \"kronk\"", got)
	}
	if got := overrides["model_providers.kronk.name"]; got != `"Kronk (local)"` {
		t.Errorf("provider name: got %q", got)
	}
	if got := overrides["model_providers.kronk.wire_api"]; got != `"responses"` {
		t.Errorf("wire_api: got %q, want \"responses\"", got)
	}
	// Auth is pinned off so Codex never demands OPENAI_API_KEY for a token-less
	// server (and we never clobber the user's real OPENAI_API_KEY).
	if got := overrides["model_providers.kronk.requires_openai_auth"]; got != "false" {
		t.Errorf("requires_openai_auth: got %q, want false", got)
	}
	base := overrides["model_providers.kronk.base_url"]
	if !strings.HasPrefix(base, `"http`) || !strings.HasSuffix(base, `/v1"`) {
		t.Errorf("base_url: got %q, want a quoted http(s) URL ending in /v1", base)
	}
	// Known context window should be forwarded as an unquoted integer.
	if got := overrides["model_context_window"]; got != "40960" {
		t.Errorf("model_context_window: got %q, want 40960", got)
	}
	// No token → no env_key.
	if _, ok := overrides["model_providers.kronk.env_key"]; ok {
		t.Errorf("env_key should be absent when KRONK_TOKEN is unset")
	}
	// Empty catalog path → no catalog override.
	if _, ok := overrides["model_catalog_json"]; ok {
		t.Errorf("model_catalog_json should be absent when no catalog path is given")
	}
}

func TestBuildCodexArgsWithCatalog(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")

	chatModels := []Model{{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Context: 40960}}

	args, err := buildCodexArgs("Qwen3-8B-Q8_0", chatModels, "/tmp/kronk-codex-catalog.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrides, _ := codexArgMap(t, args)

	if got := overrides["model_catalog_json"]; got != `"/tmp/kronk-codex-catalog.json"` {
		t.Errorf("model_catalog_json: got %q, want quoted path", got)
	}
}

func TestBuildCodexArgsWithToken(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "secret-token")

	args, err := buildCodexArgs("a/one", []Model{{ID: "a/one", Name: "a/one"}}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	overrides, _ := codexArgMap(t, args)

	if got := overrides["model_providers.kronk.env_key"]; got != `"KRONK_TOKEN"` {
		t.Errorf("env_key: got %q, want \"KRONK_TOKEN\"", got)
	}
	// Model with an unknown context window should not carry a context override.
	if _, ok := overrides["model_context_window"]; ok {
		t.Errorf("model_context_window should be absent when the window is unknown")
	}
}

func TestBuildCodexArgsRequiresModels(t *testing.T) {
	if _, err := buildCodexArgs("", nil, ""); err == nil {
		t.Errorf("expected error when no default model/models provided")
	}
}

func TestBuildCodexCatalog(t *testing.T) {
	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true, Context: 8192},
	}

	// Text model with a known context window.
	cat := buildCodexCatalog("Qwen3-8B-Q8_0", chatModels)
	models := cat["models"].([]any)
	if len(models) != 1 {
		t.Fatalf("expected 1 catalog model, got %d", len(models))
	}
	entry := models[0].(map[string]any)
	if entry["slug"] != "Qwen3-8B-Q8_0" {
		t.Errorf("slug: got %v", entry["slug"])
	}
	if entry["context_window"] != 40960 {
		t.Errorf("context_window: got %v, want 40960", entry["context_window"])
	}
	if mods := entry["input_modalities"].([]any); len(mods) != 1 || mods[0] != "text" {
		t.Errorf("input_modalities: got %v, want [text]", mods)
	}

	// Vision model → image modality added.
	visCat := buildCodexCatalog("Qwen2-VL-7B", chatModels)
	visEntry := visCat["models"].([]any)[0].(map[string]any)
	if mods := visEntry["input_modalities"].([]any); len(mods) != 2 {
		t.Errorf("vision input_modalities: got %v, want [text image]", mods)
	}

	// Unknown context window → fallback.
	fbCat := buildCodexCatalog("no/window", []Model{{ID: "no/window", Name: "no/window"}})
	fbEntry := fbCat["models"].([]any)[0].(map[string]any)
	if fbEntry["context_window"] != codexFallbackContextWindow {
		t.Errorf("fallback context_window: got %v, want %d", fbEntry["context_window"], codexFallbackContextWindow)
	}
}

func TestWriteCodexCatalog(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	cat := buildCodexCatalog("a/one", []Model{{ID: "a/one", Name: "a/one", Context: 8192}})

	path, err := writeCodexCatalog(cat)
	if err != nil {
		t.Fatalf("writeCodexCatalog: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading catalog: %v", err)
	}

	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("catalog is not valid JSON: %v", err)
	}
	if _, ok := doc["models"].([]any); !ok {
		t.Errorf("catalog missing models array")
	}
}

func TestCodexInstallerCommand(t *testing.T) {
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
			bin, args, err := codexInstallerCommand(tt.goos)
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
