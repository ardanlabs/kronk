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

func TestParseAttentionFactsQwen35MoE(t *testing.T) {
	metadata := map[string]string{
		"qwen35moe.embedding_length":        "2048",
		"qwen35moe.attention.head_count":    "16",
		"qwen35moe.attention.head_count_kv": "2",
		"qwen35moe.attention.key_length":    "256",
		"qwen35moe.attention.value_length":  "256",
		"qwen35moe.nextn_predict_layers":    "1",
		"qwen35moe.ssm.conv_kernel":         "4",
		"qwen35moe.ssm.inner_size":          "8",
		"qwen35moe.ssm.state_size":          "4",
		"qwen35moe.ssm.group_count":         "2",
	}

	got := ParseAttentionFacts(metadata, "qwen35moe", 41)
	if got.RecurrentLayers != 30 || got.FullAttentionLayers != 10 || got.NextNPredictLayers != 1 {
		t.Fatalf("layers: got recurrent=%d full=%d nextn=%d, want 30/10/1",
			got.RecurrentLayers, got.FullAttentionLayers, got.NextNPredictLayers)
	}
	if len(got.RecurrentPattern) != 41 || !got.RecurrentPattern[0] || got.RecurrentPattern[3] || got.RecurrentPattern[40] {
		t.Fatalf("recurrent pattern does not match Qwen3.5 interval-4 layout")
	}

	const wantStateBytes int64 = 4 * (3*(8+2*2*4) + 4*8)
	if got.RecurrentStateBytes != wantStateBytes {
		t.Fatalf("RecurrentStateBytes: got %d, want %d", got.RecurrentStateBytes, wantStateBytes)
	}
}

func TestParseAttentionFactsQwen35ExplicitRecurrentPattern(t *testing.T) {
	metadata := map[string]string{
		"qwen35.embedding_length":           "8",
		"qwen35.attention.head_count":       "1",
		"qwen35.attention.head_count_kv":    "1",
		"qwen35.attention.recurrent_layers": "[true false true false true]",
		"qwen35.full_attention_interval":    "2",
		"qwen35.nextn_predict_layers":       "1",
	}

	got := ParseAttentionFacts(metadata, "qwen35", 5)
	if got.RecurrentLayers != 2 || got.FullAttentionLayers != 2 {
		t.Fatalf("layers: got recurrent=%d full=%d, want 2/2", got.RecurrentLayers, got.FullAttentionLayers)
	}
	if got.RecurrentPattern[4] {
		t.Fatal("MTP layer marked recurrent")
	}
}
