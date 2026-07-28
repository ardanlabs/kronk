package vram

import "testing"

func TestCalculateSWAFull(t *testing.T) {
	input := Input{
		ContextWindow:       128,
		BlockCount:          4,
		HeadCountKV:         2,
		KeyLength:           8,
		ValueLength:         8,
		BytesPerElement:     1,
		Slots:               2,
		SlidingWindow:       16,
		SlidingWindowLayers: 3,
	}

	compact := Calculate(input)
	input.SWAFull = true
	full := Calculate(input)

	const kvPerTokenPerLayer int64 = 2 * (8 + 8)
	wantCompact := int64(2) * (128 + 3*16) * kvPerTokenPerLayer
	wantFull := int64(2) * 4 * 128 * kvPerTokenPerLayer

	if compact.SlotMemory != wantCompact {
		t.Errorf("compact SlotMemory: got %d, want %d", compact.SlotMemory, wantCompact)
	}
	if full.SlotMemory != wantFull {
		t.Errorf("full SlotMemory: got %d, want %d", full.SlotMemory, wantFull)
	}
}

func TestIsFolderURL(t *testing.T) {
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
			got := isFolderURL(tt.url)
			if got != tt.want {
				t.Errorf("isFolderURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
