package pull

import "testing"

func TestIsModelID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"Qwen3-0.6B-Q8_0", true},
		{"Qwen3.6-35B-A3B-UD-Q4_K_M", true},
		{"unsloth/Qwen3-0.6B-Q8_0", true},
		{"unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M", true},

		{"https://huggingface.co/owner/repo/resolve/main/file.gguf", false},
		{"http://huggingface.co/owner/repo/resolve/main/file.gguf", false},
		{"huggingface.co/owner/repo/resolve/main/file.gguf", false},
		{"hf.co/owner/repo/resolve/main/file.gguf", false},
		{"owner/repo/file.gguf", false},
		{"owner/repo/sub/file.gguf", false},
		{"owner/repo:Q4_K_M", false},
		{"owner/repo:Q4_K_M@revision", false},
		{"hf.co/owner/repo:Q4_K_M", false},

		{"", false},
		{"   ", false},
	}
	for _, tt := range tests {
		got := isModelID(tt.in)
		if got != tt.want {
			t.Errorf("isModelID(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
