package launch

import (
	"encoding/json"
	"testing"
)

func TestSelectChatModels(t *testing.T) {
	// Capabilities come from the catalog, keyed by the bare model name.
	caps := map[string]capability{
		"Qwen3-8B-Q8_0":              {chat: true, reasoning: true},
		"Qwen2-VL-7B":                {chat: true, vision: true},
		"embeddinggemma-300m":        {chat: false},
		"reranker":                   {chat: false},
		"Qwen3-Embedding-0.6B-Q8_0":  {chat: false},
		"Qwen3.6-35B-A3B-UD-Q8_K_XL": {chat: true},
	}

	entries := []modelListEntry{
		{ID: "Qwen3-8B-Q8_0", Validated: true},
		{ID: "Qwen2-VL-7B", Validated: true, HasProjection: true},
		{ID: "embeddinggemma-300m", Validated: true},
		{ID: "reranker", Validated: true},
		{ID: "Qwen3-Embedding-0.6B-Q8_0", Validated: true},
		// Profile variant of a chat base: chat decision follows the base
		// name and Variant must be set.
		{ID: "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", Validated: true},
		{ID: "Qwen3.6-35B-A3B-UD-Q8_K_XL", Validated: true},
		{ID: "not-validated", Validated: false},
	}

	got := selectChatModels(entries, caps)

	want := []Model{
		{ID: "Qwen2-VL-7B", Name: "Qwen2-VL-7B", Vision: true},
		{ID: "Qwen3-8B-Q8_0", Name: "Qwen3-8B-Q8_0", Reasoning: true},
		{ID: "Qwen3.6-35B-A3B-UD-Q8_K_XL", Name: "Qwen3.6-35B-A3B-UD-Q8_K_XL"},
		{ID: "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", Name: "Qwen3.6-35B-A3B-UD-Q8_K_XL/AGENT", Variant: true},
	}

	if len(got) != len(want) {
		t.Fatalf("got %d models, want %d: %+v", len(got), len(want), got)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("model[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestSelectChatModelsHeuristic covers the fallback path used when the
// catalog is unavailable (caps is nil): embedding/rerank models are
// excluded by a name-based heuristic and everything else is kept.
func TestSelectChatModelsHeuristic(t *testing.T) {
	entries := []modelListEntry{
		{ID: "Qwen3-8B-Q8_0", Validated: true},
		{ID: "bge-reranker-v2-m3-Q8_0", Validated: true, ModelFamily: "bge-reranker-v2-m3-GGUF"},
		{ID: "Qwen3-Embedding-0.6B-Q8_0", Validated: true},
	}

	got := selectChatModels(entries, nil)

	if len(got) != 1 || got[0].ID != "Qwen3-8B-Q8_0" {
		t.Fatalf("got %+v, want only Qwen3-8B-Q8_0", got)
	}
}

func TestResolveDefaultModel(t *testing.T) {
	tests := []struct {
		name       string
		requested  string
		chatModels []Model
		want       string
		wantErr    bool
	}{
		{
			name:       "empty falls back to first",
			chatModels: []Model{{ID: "a"}, {ID: "b"}},
			want:       "a",
		},
		{
			name:       "empty prefers a variant",
			chatModels: []Model{{ID: "a"}, {ID: "b/AGENT", Variant: true}},
			want:       "b/AGENT",
		},
		{
			name:       "valid requested",
			chatModels: []Model{{ID: "a"}, {ID: "b"}},
			requested:  "b",
			want:       "b",
		},
		{
			name:       "invalid requested",
			chatModels: []Model{{ID: "a"}, {ID: "b"}},
			requested:  "c",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDefaultModel(tt.requested, tt.chatModels)
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

func TestOpenCodeInstallerCommand(t *testing.T) {
	tests := []struct {
		goos    string
		wantBin string
		wantErr bool
	}{
		{goos: "windows", wantBin: "npm"},
		{goos: "darwin", wantBin: "bash"},
		{goos: "linux", wantBin: "bash"},
		{goos: "plan9", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			bin, args, err := openCodeInstallerCommand(tt.goos)
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

func TestBuildOpenCodeConfig(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")

	chatModels := []Model{
		{ID: "unsloth/Qwen3-8B-Q8_0", Name: "unsloth/Qwen3-8B-Q8_0", Reasoning: true},
		{ID: "unsloth/Qwen2-VL-7B", Name: "unsloth/Qwen2-VL-7B", Vision: true, Context: 131072},
	}

	content, err := buildOpenCodeConfig("unsloth/Qwen3-8B-Q8_0", chatModels)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}

	if cfg["model"] != "kronk/unsloth/Qwen3-8B-Q8_0" {
		t.Errorf("default model: got %v, want kronk/unsloth/Qwen3-8B-Q8_0", cfg["model"])
	}

	provider, ok := cfg["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider missing or wrong type")
	}
	kronk, ok := provider["kronk"].(map[string]any)
	if !ok {
		t.Fatalf("kronk provider missing")
	}

	entries, ok := kronk["models"].(map[string]any)
	if !ok {
		t.Fatalf("models map missing")
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 model entries, got %d", len(entries))
	}

	// Vision model should carry modalities and attachment.
	vl, ok := entries["unsloth/Qwen2-VL-7B"].(map[string]any)
	if !ok {
		t.Fatalf("vision model entry missing")
	}
	if vl["attachment"] != true {
		t.Errorf("vision model should have attachment: true")
	}
	if _, ok := vl["modalities"]; !ok {
		t.Errorf("vision model should have modalities")
	}

	// Model with a known context window should carry a limit.
	limit, ok := vl["limit"].(map[string]any)
	if !ok {
		t.Fatalf("model with known context should carry a limit")
	}
	if limit["context"].(float64) != 131072 {
		t.Errorf("limit.context: got %v, want 131072", limit["context"])
	}
	if limit["output"].(float64) != 131072/2 {
		t.Errorf("limit.output: got %v, want %d", limit["output"], 131072/2)
	}

	// Reasoning model should carry reasoning and no modalities.
	q3, ok := entries["unsloth/Qwen3-8B-Q8_0"].(map[string]any)
	if !ok {
		t.Fatalf("reasoning model entry missing")
	}
	if q3["reasoning"] != true {
		t.Errorf("reasoning model should have reasoning: true")
	}
	if _, ok := q3["modalities"]; ok {
		t.Errorf("non-vision model should not have modalities")
	}
	if _, ok := q3["limit"]; ok {
		t.Errorf("model with unknown context should not carry a limit")
	}

	// small_model must also be pinned to a local model.
	if cfg["small_model"] != "kronk/unsloth/Qwen3-8B-Q8_0" {
		t.Errorf("small_model: got %v, want kronk/unsloth/Qwen3-8B-Q8_0", cfg["small_model"])
	}

	// No token set → no apiKey in options.
	opts, _ := kronk["options"].(map[string]any)
	if _, ok := opts["apiKey"]; ok {
		t.Errorf("apiKey should be absent when KRONK_TOKEN is unset")
	}
}

func TestBuildOpenCodeConfigWithToken(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "secret-token")

	content, err := buildOpenCodeConfig("a/one", []Model{{ID: "a/one", Name: "a/one"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(content), &cfg); err != nil {
		t.Fatalf("config is not valid JSON: %v", err)
	}

	opts := cfg["provider"].(map[string]any)["kronk"].(map[string]any)["options"].(map[string]any)
	if opts["apiKey"] != "{env:KRONK_TOKEN}" {
		t.Errorf("apiKey: got %v, want {env:KRONK_TOKEN}", opts["apiKey"])
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		dash     int
		wantName string
		wantPass []string
		wantErr  bool
	}{
		{name: "no args", args: nil, dash: -1, wantName: ""},
		{name: "agent only", args: []string{"opencode"}, dash: -1, wantName: "opencode"},
		{name: "agent + passthrough", args: []string{"opencode", "--help"}, dash: 1, wantName: "opencode", wantPass: []string{"--help"}},
		{name: "trailing dash", args: []string{"opencode"}, dash: 1, wantName: "opencode"},
		{name: "dash with no agent", args: []string{"--foo"}, dash: 0, wantName: "", wantPass: []string{"--foo"}},
		{name: "extra args no dash", args: []string{"opencode", "foo"}, dash: -1, wantErr: true},
		{name: "too many before dash", args: []string{"opencode", "foo", "bar"}, dash: 2, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, pass, err := parseArgs(tt.args, tt.dash)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.wantName {
				t.Errorf("name: got %q, want %q", name, tt.wantName)
			}
			if len(pass) != len(tt.wantPass) {
				t.Fatalf("passArgs: got %v, want %v", pass, tt.wantPass)
			}
			for i := range tt.wantPass {
				if pass[i] != tt.wantPass[i] {
					t.Errorf("passArgs[%d]: got %q, want %q", i, pass[i], tt.wantPass[i])
				}
			}
		})
	}
}

func TestBuildOpenCodeConfigRequiresModels(t *testing.T) {
	if _, err := buildOpenCodeConfig("", nil); err == nil {
		t.Errorf("expected error when no default model/models provided")
	}
}
