package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestContextPoolFallbackParams(t *testing.T) {
	params := llama.ContextParams{
		NCtx:              32_768,
		NBatch:            8_192,
		NUbatch:           2_048,
		NSeqMax:           4,
		NOutputsMax:       20,
		NOutputsMaxPerSeq: 5,
	}

	got := contextPoolFallbackParams(params)

	if got.NCtx != 8_192 {
		t.Errorf("NCtx: got %d, want %d", got.NCtx, 8_192)
	}
	if got.NSeqMax != 1 {
		t.Errorf("NSeqMax: got %d, want %d", got.NSeqMax, 1)
	}
	if got.KVUnified != 0 {
		t.Errorf("KVUnified: got %d, want %d", got.KVUnified, 0)
	}
	if got.NBatch != params.NBatch {
		t.Errorf("NBatch: got %d, want %d", got.NBatch, params.NBatch)
	}
	if got.NUbatch != params.NUbatch {
		t.Errorf("NUbatch: got %d, want %d", got.NUbatch, params.NUbatch)
	}
	if got.NOutputsMax != 5 {
		t.Errorf("NOutputsMax: got %d, want %d", got.NOutputsMax, 5)
	}
	if got.NOutputsMaxPerSeq != 5 {
		t.Errorf("NOutputsMaxPerSeq: got %d, want %d", got.NOutputsMaxPerSeq, 5)
	}

	if params.NCtx != 32_768 {
		t.Errorf("original NCtx: got %d, want %d", params.NCtx, 32_768)
	}
	if params.NSeqMax != 4 {
		t.Errorf("original NSeqMax: got %d, want %d", params.NSeqMax, 4)
	}
	if params.KVUnified != 0 {
		t.Errorf("original KVUnified: got %d, want %d", params.KVUnified, 0)
	}
	if params.NOutputsMax != 20 {
		t.Errorf("original NOutputsMax: got %d, want %d", params.NOutputsMax, 20)
	}
}

func TestContextPoolFallbackParamsDefaults(t *testing.T) {
	params := llama.ContextParams{}

	got := contextPoolFallbackParams(params)

	if got.NCtx != 0 {
		t.Errorf("NCtx: got %d, want %d", got.NCtx, 0)
	}
	if got.NSeqMax != 1 {
		t.Errorf("NSeqMax: got %d, want %d", got.NSeqMax, 1)
	}
	if got.KVUnified != 0 {
		t.Errorf("KVUnified: got %d, want %d", got.KVUnified, 0)
	}
}
