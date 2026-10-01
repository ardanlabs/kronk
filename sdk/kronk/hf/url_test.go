package hf

import "testing"

func TestNormalizeDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "blob-to-resolve",
			in:   "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/blob/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
		{
			name: "resolve-unchanged",
			in:   "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
		{
			name: "shorthand",
			in:   "unsloth/Qwen3.5-35B-A3B-GGUF/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeDownloadURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeDownloadURL(%q)\n got  %q\n want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsHuggingFaceURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{name: "hugging face", url: "https://huggingface.co/owner/repo/resolve/main/model.gguf", want: true},
		{name: "short host", url: "https://hf.co/owner/repo", want: true},
		{name: "case and port", url: "HTTPS://HUGGINGFACE.CO:443/owner/repo", want: true},
		{name: "plaintext", url: "http://huggingface.co/owner/repo"},
		{name: "host suffix", url: "https://huggingface.co.example.com/owner/repo"},
		{name: "userinfo", url: "https://huggingface.co@example.com/owner/repo"},
		{name: "unrelated", url: "https://example.com/model.gguf"},
		{name: "relative", url: "owner/repo/model.gguf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHuggingFaceURL(tt.url); got != tt.want {
				t.Errorf("IsHuggingFaceURL(%q): got %t, want %t", tt.url, got, tt.want)
			}
		})
	}
}
