package launch

import (
	"strconv"
	"strings"
	"testing"
)

// copilotEnvMap turns the "KEY=value" env slice into a lookup so tests can
// assert on individual settings without depending on order.
func copilotEnvMap(t *testing.T, env []string) map[string]string {
	t.Helper()

	m := map[string]string{}
	for _, e := range env {
		key, val, ok := strings.Cut(e, "=")
		if !ok {
			t.Fatalf("env entry %q is not KEY=value", e)
		}
		m[key] = val
	}

	return m
}

func TestBuildCopilotEnv(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")

	chatModels := []Model{
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true, Context: 40960},
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
	}

	env, err := buildCopilotEnv("Qwen3-8B-Q8_0", chatModels)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := copilotEnvMap(t, env)

	if got := m["COPILOT_PROVIDER_TYPE"]; got != "openai" {
		t.Errorf("COPILOT_PROVIDER_TYPE: got %q, want openai", got)
	}
	if got := m["COPILOT_MODEL"]; got != "Qwen3-8B-Q8_0" {
		t.Errorf("COPILOT_MODEL: got %q, want Qwen3-8B-Q8_0", got)
	}
	base := m["COPILOT_PROVIDER_BASE_URL"]
	if !strings.HasPrefix(base, "http") || !strings.HasSuffix(base, "/v1") {
		t.Errorf("COPILOT_PROVIDER_BASE_URL: got %q, want an http(s) URL ending in /v1", base)
	}
	// No token → empty API key (a token-less Kronk server needs no auth).
	if got, ok := m["COPILOT_PROVIDER_API_KEY"]; !ok || got != "" {
		t.Errorf("COPILOT_PROVIDER_API_KEY: got %q (present=%v), want empty", got, ok)
	}

	// Known context window → prompt+output budgets that stay within it, with
	// a slice reserved for output.
	prompt, err := strconv.Atoi(m["COPILOT_PROVIDER_MAX_PROMPT_TOKENS"])
	if err != nil {
		t.Fatalf("MAX_PROMPT_TOKENS not an int: %v", err)
	}
	out, err := strconv.Atoi(m["COPILOT_PROVIDER_MAX_OUTPUT_TOKENS"])
	if err != nil {
		t.Fatalf("MAX_OUTPUT_TOKENS not an int: %v", err)
	}
	if out <= 0 {
		t.Errorf("MAX_OUTPUT_TOKENS: got %d, want > 0", out)
	}
	if prompt+out != 40960 {
		t.Errorf("prompt+output: got %d, want 40960 (the context window)", prompt+out)
	}
}

func TestBuildCopilotEnvWithToken(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "secret-token")

	env, err := buildCopilotEnv("a/one", []Model{{ID: "a/one", Name: "a/one"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := copilotEnvMap(t, env)

	if got := m["COPILOT_PROVIDER_API_KEY"]; got != "secret-token" {
		t.Errorf("COPILOT_PROVIDER_API_KEY: got %q, want secret-token", got)
	}
	// Model with an unknown context window carries no token budgets.
	if _, ok := m["COPILOT_PROVIDER_MAX_PROMPT_TOKENS"]; ok {
		t.Errorf("MAX_PROMPT_TOKENS should be absent when the window is unknown")
	}
	if _, ok := m["COPILOT_PROVIDER_MAX_OUTPUT_TOKENS"]; ok {
		t.Errorf("MAX_OUTPUT_TOKENS should be absent when the window is unknown")
	}
}

func TestBuildCopilotEnvRequiresModels(t *testing.T) {
	if _, err := buildCopilotEnv("", nil); err == nil {
		t.Errorf("expected error when no default model/models provided")
	}
}

func TestCopilotInstallerCommand(t *testing.T) {
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
			bin, args, err := copilotInstallerCommand(tt.goos)
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
