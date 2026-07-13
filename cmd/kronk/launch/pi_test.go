package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadExistingConfig(t *testing.T) {
	dir := t.TempDir()

	t.Run("missing file is not an error", func(t *testing.T) {
		dst := map[string]any{}
		if err := readExistingConfig(filepath.Join(dir, "nope.json"), &dst, json.Unmarshal); err != nil {
			t.Fatalf("missing file should not error, got %v", err)
		}
	})

	t.Run("empty file is not an error", func(t *testing.T) {
		p := filepath.Join(dir, "empty.json")
		if err := os.WriteFile(p, []byte("   \n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dst := map[string]any{}
		if err := readExistingConfig(p, &dst, json.Unmarshal); err != nil {
			t.Fatalf("empty file should not error, got %v", err)
		}
	})

	t.Run("valid file parses", func(t *testing.T) {
		p := filepath.Join(dir, "ok.json")
		if err := os.WriteFile(p, []byte(`{"a":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		dst := map[string]any{}
		if err := readExistingConfig(p, &dst, json.Unmarshal); err != nil {
			t.Fatalf("valid file should parse, got %v", err)
		}
		if _, ok := dst["a"]; !ok {
			t.Errorf("expected key %q to be parsed into dst", "a")
		}
	})

	t.Run("malformed file is a hard error", func(t *testing.T) {
		p := filepath.Join(dir, "bad.json")
		if err := os.WriteFile(p, []byte("{not valid json"), 0o644); err != nil {
			t.Fatal(err)
		}
		dst := map[string]any{}
		if err := readExistingConfig(p, &dst, json.Unmarshal); err == nil {
			t.Fatal("malformed file should error instead of being silently overwritten")
		}
	})
}

func TestUnmarshalJSONC(t *testing.T) {
	t.Run("plain JSON", func(t *testing.T) {
		var m map[string]any
		if err := unmarshalJSONC([]byte(`{"a":1}`), &m); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["a"] != float64(1) {
			t.Errorf("a: got %v, want 1", m["a"])
		}
	})

	t.Run("line comments and trailing commas", func(t *testing.T) {
		in := `{
  // a comment
  "providers": {
    "kronk": { "baseUrl": "http://x/v1", }, // trailing comma above and here
  },
}`
		var m map[string]any
		if err := unmarshalJSONC([]byte(in), &m); err != nil {
			t.Fatalf("JSONC should parse, got %v", err)
		}
		providers, ok := m["providers"].(map[string]any)
		if !ok || providers["kronk"] == nil {
			t.Errorf("expected providers.kronk to survive, got %v", m)
		}
	})

	t.Run("does not corrupt // inside strings", func(t *testing.T) {
		var m map[string]any
		if err := unmarshalJSONC([]byte(`{"url":"http://host/v1"}`), &m); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["url"] != "http://host/v1" {
			t.Errorf("url mangled: got %v, want http://host/v1", m["url"])
		}
	})

	t.Run("genuinely malformed still errors", func(t *testing.T) {
		var m map[string]any
		if err := unmarshalJSONC([]byte(`{"a": }`), &m); err == nil {
			t.Error("expected an error for malformed JSON even after stripping")
		}
	})
}

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
	// Force the default location regardless of any PI_CODING_AGENT_DIR in the
	// test runner's environment.
	t.Setenv("PI_CODING_AGENT_DIR", "")

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

func TestPiModelsPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Run("default location", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "")
		got, err := piModelsPath()
		if err != nil {
			t.Fatalf("piModelsPath: %v", err)
		}
		want := filepath.Join(home, ".pi", "agent", "models.json")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("honors PI_CODING_AGENT_DIR", func(t *testing.T) {
		custom := filepath.Join(home, "custom-pi")
		t.Setenv("PI_CODING_AGENT_DIR", custom)
		got, err := piModelsPath()
		if err != nil {
			t.Fatalf("piModelsPath: %v", err)
		}
		want := filepath.Join(custom, "models.json")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("expands leading tilde", func(t *testing.T) {
		t.Setenv("PI_CODING_AGENT_DIR", "~/agentdir")
		got, err := piModelsPath()
		if err != nil {
			t.Fatalf("piModelsPath: %v", err)
		}
		want := filepath.Join(home, "agentdir", "models.json")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

// TestWriteFileWithBackupPreservesOriginal verifies the backup captures the
// user's pristine original and is never clobbered by later launches, and that
// both the backup and the written file are not world/group readable (they can
// carry the user's other provider keys).
func TestWriteFileWithBackupPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := []byte(`{"user":"original"}`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatalf("seed original: %v", err)
	}

	// Two managed writes: the first backs up the original, the second must not
	// overwrite that pristine backup with an already-modified version.
	if err := writeFileWithBackup(path, []byte(`{"launch":1}`)); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeFileWithBackup(path, []byte(`{"launch":2}`)); err != nil {
		t.Fatalf("second write: %v", err)
	}

	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if string(bak) != string(original) {
		t.Errorf("backup should hold the pristine original; got %q, want %q", bak, original)
	}

	for _, p := range []string{path, path + ".bak"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s perms: got %o, want 600", p, perm)
		}
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

	install, err := loadInstall("pi")
	if err != nil {
		t.Fatalf("loadInstall: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, err := install.installerCommand(tt.goos)
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
