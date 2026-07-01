package launch

import (
	"strings"
	"testing"
)

func TestBuildClaudeEnv(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")
	t.Setenv("KRONK_WEB_API_HOST", "127.0.0.1:11435")

	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Context: 32768},
		{ID: "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", Name: "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", Variant: true, Context: 131072},
	}

	env, err := buildClaudeEnv("Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", chatModels)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := envMap(env)

	want := map[string]string{
		"ANTHROPIC_BASE_URL":             "http://127.0.0.1:11435",
		"ANTHROPIC_AUTH_TOKEN":           claudePlaceholderToken,
		"ANTHROPIC_MODEL":                "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL":  "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
		"ANTHROPIC_DEFAULT_SONNET_MODEL": "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
		"ANTHROPIC_DEFAULT_OPUS_MODEL":   "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
		"ANTHROPIC_SMALL_FAST_MODEL":     "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "131072",
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}

	// The base URL must be the server root (Claude Code appends /v1/messages).
	if strings.Contains(got["ANTHROPIC_BASE_URL"], "/v1") {
		t.Errorf("ANTHROPIC_BASE_URL should be the root, got %q", got["ANTHROPIC_BASE_URL"])
	}

	// Cloud-provider routing flags must be present and empty so an inherited
	// enabled flag cannot divert requests away from the local server.
	for _, flag := range []string{
		"CLAUDE_CODE_USE_BEDROCK",
		"CLAUDE_CODE_USE_VERTEX",
		"CLAUDE_CODE_USE_FOUNDRY",
		"CLAUDE_CODE_USE_MANTLE",
		"CLAUDE_CODE_USE_ANTHROPIC_AWS",
	} {
		v, ok := got[flag]
		if !ok {
			t.Errorf("%s should be set (to empty) to neutralize inherited routing", flag)
		}
		if v != "" {
			t.Errorf("%s should be empty, got %q", flag, v)
		}
	}
}

func TestBuildClaudeEnvToken(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "secret-token")
	t.Setenv("KRONK_WEB_API_HOST", "127.0.0.1:11435")

	env, err := buildClaudeEnv("a", []Model{{ID: "a", Name: "a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := envMap(env)

	if got["ANTHROPIC_AUTH_TOKEN"] != "secret-token" {
		t.Errorf("ANTHROPIC_AUTH_TOKEN: got %q, want secret-token", got["ANTHROPIC_AUTH_TOKEN"])
	}

	// Unknown context window must omit the override entirely.
	if _, ok := got["CLAUDE_CODE_MAX_CONTEXT_TOKENS"]; ok {
		t.Errorf("CLAUDE_CODE_MAX_CONTEXT_TOKENS should be absent when context is unknown")
	}
}

func TestBuildClaudeEnvRequiresModels(t *testing.T) {
	if _, err := buildClaudeEnv("", nil); err == nil {
		t.Errorf("expected error when no default model/models provided")
	}
}

func TestClaudeInstallerCommand(t *testing.T) {
	tests := []struct {
		goos    string
		wantBin string
		wantErr bool
	}{
		{goos: "windows", wantBin: "powershell"},
		{goos: "darwin", wantBin: "bash"},
		{goos: "linux", wantBin: "bash"},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, err := claudeInstallerCommand(tt.goos)
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

// envMap turns a []"KEY=VALUE" slice into a map for assertions.
func envMap(env []string) map[string]string {
	out := make(map[string]string, len(env))
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok {
			out[k] = v
		}
	}
	return out
}
