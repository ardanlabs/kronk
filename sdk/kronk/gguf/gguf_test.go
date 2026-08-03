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
	// head_count_kv=14, key/value length 512, 60 layers. No SWA fields
	// supplied → every layer is budgeted at full ContextWindow (legacy
	// behaviour preserved for callers that haven't wired AttentionFacts).
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

// TestCalculateKVCache_Gemma4_SWA verifies SWA-aware accounting. With 50
// of 60 layers running a 4096-token sliding window and 10 layers full
// context, the per-slot KV is dramatically smaller than the all-layers-
// full-context fallback. This is the bug behind "AGENT" configs being
// rejected: the planner predicted ~130 GB for a model that actually fits
// in ~30-50 GB.
func TestCalculateKVCache_Gemma4_SWA(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:       131072,
		BlockCount:          60,
		HeadCountKV:         4,
		KeyLength:           512,
		ValueLength:         512,
		SWAHeadCountKV:      16,
		SWAKeyLength:        256,
		SWAValueLength:      256,
		BytesPerElement:     BytesPerElementF16,
		Slots:               2,
		SlidingWindow:       4096,
		SlidingWindowLayers: 50,
		NUBatch:             2048,
		KVUnified:           true,
	}

	out := CalculateKVCache(in)

	const fullKVPerToken int64 = 4 * (512 + 512) * 2
	const swaKVPerToken int64 = 16 * (256 + 256) * 2

	wantPerSlot := int64(131072)*10*fullKVPerToken + int64(5120)*50*swaKVPerToken
	if out.KVPerSlot != wantPerSlot {
		t.Fatalf("KVPerSlot got %d, want %d", out.KVPerSlot, wantPerSlot)
	}

	wantSlotMem := wantPerSlot * 2
	if out.SlotMemory != wantSlotMem {
		t.Fatalf("SlotMemory got %d, want %d (two slots)", out.SlotMemory, wantSlotMem)
	}

	// Sanity: SWA path must produce a strictly smaller footprint than
	// the all-full-attention formula it replaces.
	fullAll := (int64(131072)*10*fullKVPerToken + int64(131072)*50*swaKVPerToken) * 2
	if out.SlotMemory >= fullAll {
		t.Fatalf("SWA result (%d) is not smaller than full-attention fallback (%d)", out.SlotMemory, fullAll)
	}
}

func TestCalculateKVCacheSharedLayers(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:   1024,
		BlockCount:      6,
		SharedKVLayers:  2,
		HeadCountKV:     1,
		KeyLength:       1,
		ValueLength:     1,
		BytesPerElement: 1,
		Slots:           1,
	}

	out := CalculateKVCache(in)
	if len(out.LayerMemory) != 4 {
		t.Fatalf("allocated layers: got %d, want 4", len(out.LayerMemory))
	}
	if out.SlotMemory != 4*1024*2 {
		t.Fatalf("SlotMemory: got %d, want %d", out.SlotMemory, 4*1024*2)
	}
}

func TestCalculateKVCacheQ8RowSize(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:   256,
		BlockCount:      1,
		HeadCountKV:     1,
		KeyLength:       128,
		ValueLength:     128,
		BytesPerElement: 1,
		TypeK:           8,
		TypeV:           8,
		Slots:           1,
	}

	out := CalculateKVCache(in)
	want := int64(256 * (136 + 136))
	if out.SlotMemory != want {
		t.Fatalf("SlotMemory: got %d, want Q8_0 row-sized %d", out.SlotMemory, want)
	}
}

func TestCalculateKVCachePerLayerHeadsAndTransposedV(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:      256,
		BlockCount:         2,
		HeadCountKV:        1,
		HeadCountKVByLayer: []int64{1, 2},
		KeyLength:          16,
		ValueLength:        16,
		BytesPerElement:    2,
		Slots:              1,
		VTransposed:        true,
	}

	out := CalculateKVCache(in)
	// K widths are 16 and 32. Transposed V uses the model-wide max width 32
	// for both layers.
	want := int64(256 * 2 * (16 + 32 + 32 + 32))
	if out.SlotMemory != want {
		t.Fatalf("SlotMemory: got %d, want %d", out.SlotMemory, want)
	}
}

func TestCalculateKVCachePadsContext(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:   300,
		BlockCount:      1,
		HeadCountKV:     1,
		KeyLength:       1,
		ValueLength:     1,
		BytesPerElement: 1,
		Slots:           2,
		KVUnified:       true,
	}

	out := CalculateKVCache(in)
	if out.SlotMemory != 768*2 {
		t.Fatalf("SlotMemory: got %d, want padded %d", out.SlotMemory, 768*2)
	}
}

func TestCalculateKVCacheQwen35Hybrid(t *testing.T) {
	recurrentPattern := make([]bool, 41)
	for layer := range int64(40) {
		recurrentPattern[layer] = (layer+1)%4 != 0
	}

	in := KVCacheInput{
		ContextWindow:        131072,
		BlockCount:           41,
		HeadCountKV:          2,
		KeyLength:            256,
		ValueLength:          256,
		BytesPerElement:      2,
		Slots:                1,
		RecurrentPattern:     recurrentPattern,
		NextNPredictLayers:   1,
		RecurrentStateBytes:  1024,
		RecurrentStateCopies: 3,
	}

	out := CalculateKVCache(in)
	wantAttention := int64(10 * 2 * (256 + 256) * 2 * 131072)
	wantRecurrent := int64(30 * 1024 * 3)
	if out.SlotMemory != wantAttention+wantRecurrent {
		t.Fatalf("SlotMemory: got %d, want %d", out.SlotMemory, wantAttention+wantRecurrent)
	}
	if out.LayerMemory[40] != 0 {
		t.Fatalf("MTP layer memory: got %d, want 0", out.LayerMemory[40])
	}
}

func TestCalculateKVCacheRecurrentMetadataFallback(t *testing.T) {
	in := KVCacheInput{
		ContextWindow:       256,
		BlockCount:          5,
		HeadCountKV:         1,
		KeyLength:           8,
		ValueLength:         8,
		BytesPerElement:     2,
		Slots:               1,
		RecurrentPattern:    []bool{true, true, true, false, false},
		NextNPredictLayers:  1,
		RecurrentStateBytes: 0,
	}

	out := CalculateKVCache(in)
	// Missing recurrent dimensions must preserve conservative full-KV
	// accounting for all four trunk layers. The appended MTP layer is still
	// known not to belong to the target context and remains excluded.
	want := int64(4 * 256 * (8 + 8) * 2)
	if out.SlotMemory != want {
		t.Fatalf("SlotMemory: got %d, want conservative fallback %d", out.SlotMemory, want)
	}
}
