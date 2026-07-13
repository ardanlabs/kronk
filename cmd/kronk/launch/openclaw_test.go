package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildOpenClawPatch(t *testing.T) {
	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true, Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	patch := buildOpenClawPatch("Qwen3-8B-Q8_0", chatModels, "http://localhost:9999/v1", "kronk", nil)

	models := patch["models"].(map[string]any)
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
	defaults := patch["agents"].(map[string]any)["defaults"].(map[string]any)
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

func TestBuildOpenClawPatchPrunesStaleRefs(t *testing.T) {
	chatModels := []Model{{ID: "current", Name: "current"}}

	// One genuinely stale ref, plus one that overlaps with a current model
	// (which must never be turned into a delete).
	staleRefs := []string{"kronk/gone", "kronk/current"}

	patch := buildOpenClawPatch("current", chatModels, "http://x/v1", "kronk", staleRefs)

	allow := patch["agents"].(map[string]any)["defaults"].(map[string]any)["models"].(map[string]any)

	// Stale ref is present with a nil value → "config patch" deletes it.
	gone, ok := allow["kronk/gone"]
	if !ok {
		t.Fatalf("expected kronk/gone to be present as a null delete")
	}
	if gone != nil {
		t.Errorf("kronk/gone: got %v, want nil (delete)", gone)
	}

	// Current model keeps its object entry, never a null delete.
	cur, ok := allow["kronk/current"]
	if !ok {
		t.Fatalf("expected kronk/current to be present")
	}
	if cur == nil {
		t.Errorf("kronk/current must not be a null delete")
	}
}

// TestWriteOpenClawConfigIntegration exercises the real "openclaw config patch"
// path against a throwaway HOME. It verifies that our provider lands in the
// file OpenClaw actually reads, that the user's own config is preserved, and
// that a JSON5 existing config (comments + trailing commas) does not break the
// merge. It is skipped when the openclaw binary is not installed.
func TestWriteOpenClawConfigIntegration(t *testing.T) {
	bin, err := exec.LookPath("openclaw")
	if err != nil {
		t.Skip("openclaw not installed; skipping integration test")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("KRONK_TOKEN", "")

	dir := filepath.Join(home, ".openclaw")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "openclaw.json")

	// A pre-existing JSON5 config (comment + trailing comma) with a user
	// provider that must survive the merge and a stale kronk allowlist entry
	// (from an earlier launch) that must be pruned.
	existing := `{
  // user's own settings (JSON5)
  "models": { "providers": { "userprov": { "baseUrl": "http://user/v1", "apiKey": "k", "api": "openai-completions", "models": [ { "id": "user-model", "name": "user-model" } ] } } },
  "agents": { "defaults": { "models": { "kronk/Old-Model": {}, "userprov/user-model": {} } } },
}
`
	if err := os.WriteFile(path, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing: %v", err)
	}

	models := []Model{{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Context: 40960}}
	if err := writeOpenClawConfig(bin, nil, "Qwen3-8B-Q8_0", models); err != nil {
		t.Fatalf("writeOpenClawConfig: %v", err)
	}

	// The kronk provider is present in the file OpenClaw reads.
	if got := openclawConfigGet(t, bin, home, "models.providers.kronk.baseUrl"); got == "" {
		t.Errorf("kronk provider not written")
	}
	// The user's own provider survived the merge.
	if got := openclawConfigGet(t, bin, home, "models.providers.userprov.baseUrl"); got != "http://user/v1" {
		t.Errorf("user provider not preserved: got %q", got)
	}
	// The primary model is our default.
	if got := openclawConfigGet(t, bin, home, "agents.defaults.model.primary"); got != "kronk/Qwen3-8B-Q8_0" {
		t.Errorf("primary: got %q, want kronk/Qwen3-8B-Q8_0", got)
	}
	// The stale kronk allowlist entry was pruned.
	if got := openclawConfigGet(t, bin, home, "agents.defaults.models.kronk/Old-Model"); got != "" {
		t.Errorf("stale kronk/Old-Model should have been pruned, got %q", got)
	}
	// The user's own allowlist entry survived.
	if got := openclawConfigGet(t, bin, home, "agents.defaults.models.userprov/user-model"); got == "" {
		t.Errorf("user allowlist entry should be preserved")
	}
	// The current model's allowlist entry is present.
	if got := openclawConfigGet(t, bin, home, "agents.defaults.models.kronk/Qwen3-8B-Q8_0"); got == "" {
		t.Errorf("current model allowlist entry missing")
	}
}

// openclawConfigGet returns a single config value via "openclaw config get",
// trimmed of surrounding whitespace, or "" when the path is missing.
func openclawConfigGet(t *testing.T, bin, home, path string) string {
	t.Helper()

	cmd := exec.Command(bin, "config", "get", path)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	s := string(out)
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}

	return s
}

func TestWriteOpenClawConfigRequiresModels(t *testing.T) {
	if err := writeOpenClawConfig("openclaw", nil, "", nil); err == nil {
		t.Errorf("expected error when no models provided")
	}
}

func TestOpenClawScopeArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr bool
	}{
		{name: "none", args: []string{"chat"}, want: nil},
		{name: "profile space", args: []string{"--profile", "foo", "chat"}, want: []string{"--profile", "foo"}},
		{name: "profile equals", args: []string{"--profile=foo", "chat"}, want: []string{"--profile=foo"}},
		{name: "dev", args: []string{"--dev", "chat"}, want: []string{"--dev"}},
		{name: "dev and profile", args: []string{"--dev", "--profile", "foo"}, want: []string{"--dev", "--profile", "foo"}},
		{name: "profile missing value", args: []string{"--profile"}, wantErr: true},
		{name: "container rejected", args: []string{"--container", "box", "chat"}, wantErr: true},
		{name: "container equals rejected", args: []string{"--container=box"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := openclawScopeArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (scope=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("scope: got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("scope[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOpenClawRunArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "no args defaults to chat under launch session", args: nil, want: []string{"chat", "--session", "kronk"}},
		{name: "profile only appends chat under launch session", args: []string{"--profile", "work"}, want: []string{"--profile", "work", "chat", "--session", "kronk"}},
		{name: "profile equals only appends chat under launch session", args: []string{"--profile=work"}, want: []string{"--profile=work", "chat", "--session", "kronk"}},
		{name: "dev only appends chat under launch session", args: []string{"--dev"}, want: []string{"--dev", "chat", "--session", "kronk"}},
		{name: "dev and profile appends chat under launch session", args: []string{"--dev", "--profile", "work"}, want: []string{"--dev", "--profile", "work", "chat", "--session", "kronk"}},
		{name: "explicit subcommand is verbatim", args: []string{"--profile", "work", "chat"}, want: []string{"--profile", "work", "chat"}},
		{name: "explicit chat with own session is verbatim", args: []string{"chat", "--session", "mine"}, want: []string{"chat", "--session", "mine"}},
		{name: "other subcommand is verbatim", args: []string{"config", "get", "x"}, want: []string{"config", "get", "x"}},
		{name: "help stays verbatim", args: []string{"--help"}, want: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// scope mirrors what Run computes before calling openclawRunArgs.
			scope, err := openclawScopeArgs(tt.args)
			if err != nil {
				t.Fatalf("openclawScopeArgs: %v", err)
			}

			got := openclawRunArgs(tt.args, scope)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("arg[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOpenClawEnvStripsContainer(t *testing.T) {
	t.Setenv("OPENCLAW_CONTAINER", "my-box")
	t.Setenv("OPENCLAW_HOME", "/tmp/oclhome")

	for _, kv := range openclawEnv() {
		if strings.HasPrefix(kv, "OPENCLAW_CONTAINER=") {
			t.Errorf("OPENCLAW_CONTAINER should be stripped, found %q", kv)
		}
	}

	// Other OPENCLAW_ vars (and the rest of the environment) must survive.
	var sawHome bool
	for _, kv := range openclawEnv() {
		if kv == "OPENCLAW_HOME=/tmp/oclhome" {
			sawHome = true
		}
	}
	if !sawHome {
		t.Errorf("OPENCLAW_HOME should be preserved")
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

	install, err := loadInstall("openclaw")
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
