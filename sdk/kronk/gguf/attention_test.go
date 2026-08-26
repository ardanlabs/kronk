package gguf

import "testing"

func TestCountSWALayers(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    int64
	}{
		{
			name:    "gemma4-60-layers",
			pattern: "[true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false]",
			want:    50,
		},
		{
			name:    "all-true",
			pattern: "[true true true]",
			want:    3,
		},
		{
			name:    "all-false",
			pattern: "[false false false]",
			want:    0,
		},
		{
			name:    "empty",
			pattern: "[]",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountSWALayers(tt.pattern)
			if got != tt.want {
				t.Errorf("CountSWALayers() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestParseAttentionFactsGemma4(t *testing.T) {
	metadata := map[string]string{
		"gemma4.embedding_length":                 "5120",
		"gemma4.attention.head_count":             "10",
		"gemma4.attention.head_count_kv":          "[8 8 8 8 8 2]",
		"gemma4.attention.key_length":             "512",
		"gemma4.attention.value_length":           "512",
		"gemma4.attention.key_length_swa":         "256",
		"gemma4.attention.value_length_swa":       "256",
		"gemma4.attention.sliding_window":         "1024",
		"gemma4.attention.sliding_window_pattern": "[true, true, true, true, true, false]",
	}

	got := ParseAttentionFacts(metadata, "gemma4", 6)
	if got.SlidingWindowLayers != 5 || got.FullAttentionLayers != 1 {
		t.Fatalf("layers: got SWA=%d full=%d, want 5/1", got.SlidingWindowLayers, got.FullAttentionLayers)
	}
	if got.SWAHeadCountKV != 8 || got.FullHeadCountKV != 2 {
		t.Fatalf("KV heads: got SWA=%d full=%d, want 8/2", got.SWAHeadCountKV, got.FullHeadCountKV)
	}
	if got.SWAKeyLength != 256 || got.SWAValueLength != 256 {
		t.Fatalf("SWA dimensions: got %d/%d, want 256/256", got.SWAKeyLength, got.SWAValueLength)
	}
	if got.FullKeyLength != 512 || got.FullValueLength != 512 {
		t.Fatalf("full dimensions: got %d/%d, want 512/512", got.FullKeyLength, got.FullValueLength)
	}
}
