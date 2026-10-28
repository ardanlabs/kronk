package vram

import "testing"

func TestDecisionExecutionEstimateScope(t *testing.T) {
	for _, tt := range []struct {
		architecture, protocol, causal string
		wholePrompt, noCache           bool
	}{
		{"lfm3", "lfm3-d1", "", true, true},
		{"lfm3", "lfm3-d1", "true", true, true},
		{"lfm3", "lfm3-d1", "false", true, true},
		{"lfm3", "", "false", false, true},
		{"lfm3", "unknown", "", false, false},
		{"lfm3", "laya", "", false, false},
		{"lfm3", "clef", "", false, false},
		{"lfm3", "vllm-decision2", "", false, false},
		{"llama", "vllm-d3", "", false, false},
		{"llama", "vllm-decision2", "", false, false},
		{"llama", "lfm3-d1", "", false, false},
	} {
		t.Run(tt.architecture+"/"+tt.protocol+"/"+tt.causal, func(t *testing.T) {
			prefix := tt.architecture + "."
			metadata := map[string]string{
				"general.architecture":             tt.architecture,
				prefix + "block_count":             "4",
				prefix + "embedding_length":        "256",
				prefix + "vocab_size":              "1024",
				prefix + "attention.head_count":    "8",
				prefix + "attention.head_count_kv": "4",
				prefix + "attention.key_length":    "32",
				prefix + "attention.value_length":  "32",
				prefix + "decision.type":           tt.protocol,
			}
			if tt.architecture == "lfm3" {
				metadata[prefix+"attention.head_count_kv"] = "[0 2 0 4]"
				metadata[prefix+"attention.sliding_window_pattern"] = "[false true false false]"
				metadata[prefix+"attention.sliding_window"] = "512"
				metadata[prefix+"shortconv.l_cache"] = "3"
				if tt.causal != "" {
					metadata[prefix+"attention.causal"] = tt.causal
				}
			}
			cfg := Config{ContextWindow: 4096, Slots: 2, NUBatch: 128, BytesPerElement: 2}
			result, err := buildFromMetadata(metadata, nil, 1<<30, cfg)
			if err != nil {
				t.Fatal(err)
			}
			wantBatch, wantSlots, wantRows := int64(128), int64(2), int64(0)
			if tt.architecture == "llama" {
				wantRows = 2
			}
			if tt.wholePrompt {
				wantBatch, wantSlots, wantRows = 4096, 1, 1
			}
			if result.Input.NUBatch != wantBatch || result.Input.Slots != wantSlots || result.Input.OutputRows != wantRows || result.Input.NoPersistentCache != tt.noCache {
				t.Errorf("execution estimate: got batch/slots/rows/noCache %d/%d/%d/%t, want %d/%d/%d/%t", result.Input.NUBatch, result.Input.Slots, result.Input.OutputRows, result.Input.NoPersistentCache, wantBatch, wantSlots, wantRows, tt.noCache)
			}
			if tt.noCache && result.SlotMemory != 0 {
				t.Errorf("SlotMemory: got %d, want 0", result.SlotMemory)
			}
		})
	}
}
