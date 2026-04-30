package models

import (
	"testing"
)

func TestIsHuggingFaceFolderURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "shorthand-with-tag",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "shorthand-with-revision",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M@main",
			want: false,
		},
		{
			name: "shorthand-with-hf-prefix",
			url:  "hf.co/bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "full-gguf-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "short-form-gguf",
			url:  "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "blob-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/blob/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "folder-tree-url",
			url:  "https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF/tree/main/UD-Q5_K_XL",
			want: true,
		},
		{
			name: "short-form-folder",
			url:  "owner/repo/subfolder",
			want: true,
		},
		{
			name: "owner-repo-only",
			url:  "owner/repo",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHuggingFaceFolderURL(tt.url)
			if got != tt.want {
				t.Errorf("isHuggingFaceFolderURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

func TestNormalizeHuggingFaceDownloadURL(t *testing.T) {
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
			got := NormalizeHuggingFaceDownloadURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeHuggingFaceDownloadURL(%q)\n got  %q\n want %q", tt.in, got, tt.want)
			}
		})
	}
}
