package gguf

import "testing"

func TestParseInt64OrArrayAvg(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		want    int64
		wantErr bool
	}{
		{name: "scalar", val: "8", want: 8},
		// 8*5 + 2 = 42, 42/6 = 7 (integer division).
		{name: "space-array", val: "[8 8 8 8 8 2]", want: 7},
		{name: "comma-array (llama.cpp gguf_kv_to_str)", val: "[8, 8, 8, 8, 8, 2]", want: 7},
		// 8 + 8 + 8 + 2 = 26, 26/4 = 6.
		{name: "mixed-whitespace-array", val: "[ 8 ,8 , 8 , 2 ]", want: 6},
		{name: "single-element-array", val: "[16]", want: 16},
		{name: "empty-array", val: "[]", wantErr: true},
		{name: "missing-key", val: "", wantErr: true},
		{name: "non-int", val: "abc", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			md := map[string]string{}
			if tc.val != "" {
				md["k"] = tc.val
			}

			got, err := ParseInt64OrArrayAvg(md, "k")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got value=%d", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// TestParseInt64OrArrayAvg_Gemma4 exercises the per-layer head_count_kv
// array that gemma3/gemma4 ships with. llama.cpp's gguf_kv_to_str returns
// false for ARRAY-typed values, so the SDK's metadata enumeration
// (toModelInfo) only ever sees the space-separated rendering produced by
// our own GGUF parser. Both paths must average to the same 14 (16*5/6 +
// 4/6 = 14).
func TestParseInt64OrArrayAvg_Gemma4(t *testing.T) {
	const layout = "[16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4 16 16 16 16 16 4]"
	md := map[string]string{"gemma4.attention.head_count_kv": layout}

	got, err := ParseInt64OrArrayAvg(md, "gemma4.attention.head_count_kv")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 50 layers at 16, 10 layers at 4: (50*16 + 10*4)/60 = 840/60 = 14.
	if got != 14 {
		t.Fatalf("got %d, want 14", got)
	}
}

func TestResolveKVLengths(t *testing.T) {
	t.Run("explicit", func(t *testing.T) {
		md := map[string]string{
			"gemma4.attention.key_length":   "512",
			"gemma4.attention.value_length": "512",
		}
		k, v, err := ResolveKVLengths(md, "gemma4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if k != 512 || v != 512 {
			t.Fatalf("got k=%d v=%d, want 512/512", k, v)
		}
	})

	t.Run("derived-from-embedding", func(t *testing.T) {
		md := map[string]string{
			"qwen2.embedding_length":        "4096",
			"qwen2.attention.head_count":    "32",
			"qwen2.attention.head_count_kv": "8",
		}
		k, v, err := ResolveKVLengths(md, "qwen2")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 4096/32 = 128.
		if k != 128 || v != 128 {
			t.Fatalf("got k=%d v=%d, want 128/128", k, v)
		}
	})

	t.Run("missing-everything", func(t *testing.T) {
		_, _, err := ResolveKVLengths(map[string]string{}, "missing")
		if err == nil {
			t.Fatalf("want error, got nil")
		}
	})
}

func TestBytesPerElement(t *testing.T) {
	tests := []struct {
		name string
		id   int32
		want int64
	}{
		{name: "f32", id: 50, want: 4},
		{name: "f16", id: 1, want: 2},
		{name: "bf16", id: 30, want: 2},
		{name: "q8_0", id: 8, want: 1},
		{name: "q4_0", id: 2, want: 1},
		{name: "auto-falls-back-to-f16", id: 0, want: 2},
		{name: "unknown-falls-back-to-f16", id: 999, want: 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := BytesPerElement(tc.id); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

func TestMaxBytesPerElement(t *testing.T) {
	// Q8_0 (1 byte) vs F16 (2 bytes) -> 2.
	if got := MaxBytesPerElement(8, 1); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
	// F16 vs F16.
	if got := MaxBytesPerElement(1, 1); got != 2 {
		t.Fatalf("got %d, want 2", got)
	}
}

func TestCalculateKVCache_Gemma4(t *testing.T) {
	// Gemma4 31B at 32K context, single slot, F16 cache, averaged
	// head_count_kv=14, key/value length 512, 60 layers.
	in := KVCacheInput{
		ContextWindow:   32768,
		BlockCount:      60,
		HeadCountKV:     14,
		KeyLength:       512,
		ValueLength:     512,
		BytesPerElement: BytesPerElementF16,
		Slots:           1,
	}

	out := CalculateKVCache(in)

	// Per token per layer: 14 * (512+512) * 2 = 28_672 bytes.
	if out.KVPerTokenPerLayer != 28_672 {
		t.Fatalf("KVPerTokenPerLayer got %d, want 28672", out.KVPerTokenPerLayer)
	}

	// Per slot: 32768 * 60 * 28672.
	wantPerSlot := int64(32768) * 60 * 28_672
	if out.KVPerSlot != wantPerSlot {
		t.Fatalf("KVPerSlot got %d, want %d", out.KVPerSlot, wantPerSlot)
	}

	if out.SlotMemory != wantPerSlot {
		t.Fatalf("SlotMemory got %d, want %d (single slot)", out.SlotMemory, wantPerSlot)
	}
}
