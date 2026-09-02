package model

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestEmbeddedMTPContextParamsCapsOutputs(t *testing.T) {
	target := llama.ContextParams{
		NCtx:               4096,
		NBatch:             2064,
		NUbatch:            2064,
		NSeqMax:            4,
		NRsSeq:             3,
		NThreads:           6,
		NThreadsBatch:      8,
		FlashAttentionType: llama.FlashAttentionTypeAuto,
		TypeK:              llama.GGMLTypeF16,
		TypeV:              llama.GGMLTypeF16,
		Offload_kqv:        1,
		OpOffload:          1,
		KVUnified:          1,
		SwaFull:            1,
		RopeScalingType:    llama.RopeScalingTypeYARN,
		RopeFreqBase:       1_000_000,
		RopeFreqScale:      0.25,
		YarnExtFactor:      1.5,
		YarnAttnFactor:     1.25,
		YarnBetaFast:       32,
		YarnBetaSlow:       1,
		YarnOrigCtx:        32_768,
	}

	got := embeddedMTPContextParams(llama.ContextParams{}, target)
	if got.CtxType != llama.ContextTypeMTP {
		t.Errorf("CtxType = %d, want %d", got.CtxType, llama.ContextTypeMTP)
	}
	if got.NOutputsMax != target.NSeqMax {
		t.Errorf("NOutputsMax = %d, want %d", got.NOutputsMax, target.NSeqMax)
	}
	if got.NOutputsMaxPerSeq != 1 {
		t.Errorf("NOutputsMaxPerSeq = %d, want 1", got.NOutputsMaxPerSeq)
	}
	if got.NCtx != target.NCtx || got.NBatch != target.NBatch || got.NUbatch != target.NUbatch || got.NSeqMax != target.NSeqMax {
		t.Errorf("context sizing = (%d, %d, %d, %d), want (%d, %d, %d, %d)",
			got.NCtx, got.NBatch, got.NUbatch, got.NSeqMax,
			target.NCtx, target.NBatch, target.NUbatch, target.NSeqMax)
	}
	if got.RopeScalingType != target.RopeScalingType || got.RopeFreqBase != target.RopeFreqBase || got.RopeFreqScale != target.RopeFreqScale {
		t.Errorf("RoPE params = (%d, %g, %g), want (%d, %g, %g)",
			got.RopeScalingType, got.RopeFreqBase, got.RopeFreqScale,
			target.RopeScalingType, target.RopeFreqBase, target.RopeFreqScale)
	}
	if got.YarnExtFactor != target.YarnExtFactor || got.YarnAttnFactor != target.YarnAttnFactor || got.YarnBetaFast != target.YarnBetaFast || got.YarnBetaSlow != target.YarnBetaSlow || got.YarnOrigCtx != target.YarnOrigCtx {
		t.Errorf("YaRN params = (%g, %g, %g, %g, %d), want (%g, %g, %g, %g, %d)",
			got.YarnExtFactor, got.YarnAttnFactor, got.YarnBetaFast, got.YarnBetaSlow, got.YarnOrigCtx,
			target.YarnExtFactor, target.YarnAttnFactor, target.YarnBetaFast, target.YarnBetaSlow, target.YarnOrigCtx)
	}
}

func TestSpeculativeContextCount(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want int64
	}{
		{name: "target only", cfg: Config{}, want: 1},
		{name: "separate draft estimated independently", cfg: Config{PtrDraftModel: &DraftModelConfig{ModelFiles: []string{"draft.gguf"}}}, want: 1},
		{name: "companion MTP", cfg: Config{MTPDrafterFile: "mtp.gguf"}, want: 2},
		{name: "disabled", cfg: Config{Speculation: SpeculationDisabled, MTPDrafterFile: "mtp.gguf"}, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SpeculativeContextCount(tt.cfg); got != tt.want {
				t.Errorf("SpeculativeContextCount: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestMetadataHasMTP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{"no MTP key", map[string]string{"general.architecture": "qwen35moe"}, false},
		{"zero layers", map[string]string{"qwen35moe.nextn_predict_layers": "0"}, false},
		{"positive layers", map[string]string{"qwen35moe.nextn_predict_layers": "1"}, true},
		{"malformed layers", map[string]string{"qwen35moe.nextn_predict_layers": "invalid"}, false},
		{"architecture-specific key", map[string]string{"cohere2moe.nextn_predict_layers": " 2 "}, true},
		{"matching text within key", map[string]string{"vendor.optional_nextn_predict_layers.count": "3"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataHasMTP(tt.metadata)
			if got != tt.want {
				t.Errorf("metadataHasMTP() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMetadataHasAssistantMTP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{"assistant with MTP", map[string]string{"general.architecture": "gemma4-assistant", "gemma4.nextn_predict_layers": "1"}, true},
		{"assistant without MTP", map[string]string{"general.architecture": "gemma4-assistant"}, false},
		{"target with MTP", map[string]string{"general.architecture": "gemma4", "gemma4.nextn_predict_layers": "1"}, false},
		{"malformed MTP", map[string]string{"general.architecture": "gemma4-assistant", "gemma4.nextn_predict_layers": "invalid"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataHasAssistantMTP(tt.metadata)
			if got != tt.want {
				t.Errorf("metadataHasAssistantMTP() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestModelFilesLoadMTPUsesFirstShard(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "model-00001-of-00002.gguf")
	second := filepath.Join(dir, "model-00002-of-00002.gguf")

	writeTestGGUF(t, first, map[string]uint32{"qwen35moe.nextn_predict_layers": 1})
	writeTestGGUF(t, second, nil)

	got, err := modelFilesLoadMTP([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesLoadMTP() error = %v", err)
	}
	if !got {
		t.Errorf("modelFilesLoadMTP() = false, want true")
	}

	writeTestGGUF(t, first, nil)
	writeTestGGUF(t, second, map[string]uint32{"qwen35moe.nextn_predict_layers": 1})

	got, err = modelFilesLoadMTP([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesLoadMTP() error = %v", err)
	}
	if got {
		t.Errorf("modelFilesLoadMTP() = true, want false")
	}
}

func writeTestGGUF(t *testing.T, file string, metadata map[string]uint32) {
	t.Helper()

	var data bytes.Buffer
	values := []any{gguf.Magic, uint32(3), uint64(0), uint64(len(metadata))}
	for _, value := range values {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() error = %v", err)
		}
	}

	for key, value := range metadata {
		if err := binary.Write(&data, binary.LittleEndian, uint64(len(key))); err != nil {
			t.Fatalf("binary.Write() key length error = %v", err)
		}
		if _, err := data.WriteString(key); err != nil {
			t.Fatalf("WriteString() error = %v", err)
		}
		if err := binary.Write(&data, binary.LittleEndian, gguf.MetadataValueTypeUInt32); err != nil {
			t.Fatalf("binary.Write() value type error = %v", err)
		}
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() value error = %v", err)
		}
	}

	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
