package launch

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		dash      int
		wantModel string
		want      []string
		wantErr   bool
	}{
		{name: "missing agent", dash: -1, wantErr: true},
		{name: "missing model", args: []string{"opencode"}, dash: -1, wantErr: true},
		{name: "opencode", args: []string{"opencode", "model"}, dash: -1, wantModel: "model"},
		{name: "case insensitive", args: []string{"OpenCode", "model"}, dash: -1, wantModel: "model"},
		{name: "unsupported agent", args: []string{"codex"}, dash: -1, wantErr: true},
		{name: "argument needs separator", args: []string{"opencode", "model", "--help"}, dash: -1, wantErr: true},
		{name: "passthrough", args: []string{"opencode", "model", "--help"}, dash: 2, wantModel: "model", want: []string{"--help"}},
		{name: "too little before separator", args: []string{"opencode", "--help"}, dash: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, got, err := parseArgs(tt.args, tt.dash)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if model != tt.wantModel {
				t.Errorf("model: got %q, want %q", model, tt.wantModel)
			}
			if strings.Join(got, "\x00") != strings.Join(tt.want, "\x00") {
				t.Errorf("arguments: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrepareOpenCode(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "secret")

	root := t.TempDir()
	parent := []string{
		"HOME=/home/test",
		"OPENCODE_CONFIG=/user/config.json",
		"OPENCODE_CONFIG_DIR=/user/config",
		"OPENCODE_CONFIG_CONTENT={\"user\":true}",
		"XDG_CONFIG_HOME=/user/xdg",
	}

	available := []string{"owner/custom-model/AGENT"}
	workspace, env, err := prepareOpenCode(root, parent, available, "http://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("prepareOpenCode: %v", err)
	}

	if workspace != filepath.Join(root, "workspace") {
		t.Errorf("workspace: got %q, want %q", workspace, filepath.Join(root, "workspace"))
	}

	for _, path := range []string{
		filepath.Join(root, "config", "opencode", "opencode.jsonc"),
		filepath.Join(root, "config", "opencode", "tui.jsonc"),
		filepath.Join(root, "config", "opencode", "AGENTS.md"),
		filepath.Join(root, "config", "opencode", "skills", "writing-go", "SKILL.md"),
		filepath.Join(root, "config", "opencode", "skills", "kronk-mcp", "SKILL.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected materialized default %s: %v", path, err)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "data", "opencode", "auth.json")); !os.IsNotExist(err) {
		t.Errorf("auth.json should not be materialized; runtime credentials come from KRONK_TOKEN")
	}

	got := envMap(env)
	if got["HOME"] != filepath.Join(root, "home") {
		t.Errorf("HOME: got %q, want %q", got["HOME"], filepath.Join(root, "home"))
	}
	if got["XDG_CONFIG_HOME"] != filepath.Join(root, "config") {
		t.Errorf("XDG_CONFIG_HOME: got %q", got["XDG_CONFIG_HOME"])
	}
	if _, ok := got["OPENCODE_CONFIG"]; ok {
		t.Errorf("inherited OPENCODE_CONFIG should be removed")
	}
	if _, ok := got["OPENCODE_CONFIG_DIR"]; ok {
		t.Errorf("inherited OPENCODE_CONFIG_DIR should be removed")
	}
	if got["OPENCODE_AUTH_CONTENT"] != "{}" {
		t.Errorf("OPENCODE_AUTH_CONTENT: got %q, want {}", got["OPENCODE_AUTH_CONTENT"])
	}
	if got["OPENCODE_DISABLE_PROJECT_CONFIG"] != "1" {
		t.Errorf("OPENCODE_DISABLE_PROJECT_CONFIG: got %q, want 1", got["OPENCODE_DISABLE_PROJECT_CONFIG"])
	}

	var content map[string]any
	if err := json.Unmarshal([]byte(got["OPENCODE_CONFIG_CONTENT"]), &content); err != nil {
		t.Fatalf("runtime config is not valid JSON: %v", err)
	}
	provider := content["provider"].(map[string]any)["kronk"].(map[string]any)
	options := provider["options"].(map[string]any)
	if options["baseURL"] != "http://127.0.0.1:9999/v1" {
		t.Errorf("baseURL: got %v, want http://127.0.0.1:9999/v1", options["baseURL"])
	}
	if options["apiKey"] != "{env:KRONK_TOKEN}" {
		t.Errorf("apiKey: got %v, want {env:KRONK_TOKEN}", options["apiKey"])
	}

	data, err := os.ReadFile(filepath.Join(root, "config", "opencode", "opencode.jsonc"))
	if err != nil {
		t.Fatalf("reading filtered OpenCode config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("filtered OpenCode config is not valid JSON: %v", err)
	}
	entries, err := modelEntries(config)
	if err != nil {
		t.Fatalf("modelEntries: %v", err)
	}
	if len(entries) != 1 || entries[available[0]] == nil {
		t.Errorf("models: got %v, want only %q", entries, available[0])
	}
	if config["model"] != "kronk/"+available[0] {
		t.Errorf("model: got %v, want kronk/%s", config["model"], available[0])
	}
	mcp := config["mcp"].(map[string]any)
	if _, ok := mcp["kronk"]; ok {
		t.Errorf("temporary config should not reference the disabled Kronk MCP service")
	}
}

func TestFilterOpenCodeModelsPreservesOutputLimit(t *testing.T) {
	configDir := t.TempDir()
	modelID := "unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT"

	if err := filterOpenCodeModels(configDir, []string{modelID}); err != nil {
		t.Fatalf("filterOpenCodeModels: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(configDir, "opencode.jsonc"))
	if err != nil {
		t.Fatalf("reading filtered OpenCode config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("filtered OpenCode config is not valid JSON: %v", err)
	}
	entries, err := modelEntries(config)
	if err != nil {
		t.Fatalf("modelEntries: %v", err)
	}
	model := entries[modelID].(map[string]any)
	limit := model["limit"].(map[string]any)
	if got, want := limit["output"], float64(8192); got != want {
		t.Errorf("output limit: got %v, want %v", got, want)
	}
}

func TestOpenCodeInstaller(t *testing.T) {
	tests := []struct {
		goos    string
		wantBin string
		wantErr bool
	}{
		{goos: "darwin", wantBin: "bash"},
		{goos: "linux", wantBin: "bash"},
		{goos: "windows", wantBin: "npm"},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, hint, deps, err := openCodeInstaller(tt.goos)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bin != tt.wantBin {
				t.Errorf("binary: got %q, want %q", bin, tt.wantBin)
			}
			if len(args) == 0 || hint == "" || len(deps) == 0 {
				t.Errorf("incomplete installer: args=%v hint=%q deps=%v", args, hint, deps)
			}
		})
	}
}

func TestNormalizeServerURL(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		want    string
		wantErr bool
	}{
		{name: "default", want: "http://localhost:11435"},
		{name: "host and port", host: "127.0.0.1:9000", want: "http://127.0.0.1:9000"},
		{name: "https", host: "https://kronk.example.com:443", want: "https://kronk.example.com:443"},
		{name: "missing port", host: "localhost", wantErr: true},
		{name: "path", host: "http://localhost:11435/v1", wantErr: true},
		{name: "unsupported scheme", host: "ftp://localhost:11435", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeServerURL(tt.host)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVerifyServer(t *testing.T) {
	const downloadedID = "owner/model/AGENT"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path: got %q, want /v1/models", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"object":"list","data":[{"id":%q}]}`, downloadedID)
	}))
	defer server.Close()

	if err := verifyServer(context.Background(), server.URL, "owner/model/AGENT"); err != nil {
		t.Fatalf("verifyServer: %v", err)
	}
	if err := verifyServer(context.Background(), server.URL, "owner/missing/AGENT"); err == nil {
		t.Fatal("expected unavailable model error, got nil")
	}
}

func envMap(env []string) map[string]string {
	values := make(map[string]string, len(env))
	for _, value := range env {
		key, val, ok := strings.Cut(value, "=")
		if ok {
			values[key] = val
		}
	}
	return values
}
