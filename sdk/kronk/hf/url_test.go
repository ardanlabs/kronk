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
