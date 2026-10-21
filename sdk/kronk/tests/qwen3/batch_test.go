package qwen3_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

func Test_BatchChatConcurrent(t *testing.T) {
	// On ROCm, two sequences sharing one llama_decode batch can return
	// corrupted logits: https://github.com/ggml-org/llama.cpp/issues/28537
	// Confirmed across ROCm 7.2.4 and 10.0 at llama.cpp b10798:
	// runs 33929435557 and 33931215130 failed with empty responses, while
	// 33928022827 passed. Removal needs consecutive green runs, not one.
	testlib.SkipOnBackends(t, "two sequences sharing one decode batch come back with corrupted logits", "rocm")

	words := []string{"ALPHA", "BRAVO", "COPPER", "DELTA", "ECHO", "FOXTROT", "GOLF", "HOTEL", "INDIGO", "JULIET"}
	// Admit the complete ten-request burst; this tests inference isolation,
	// not queue rejection. Disable IMC to separate batching from cached replay.
	cfg := testlib.CfgThinkToolChat()
	cfg.PtrQueueDepth = new((len(words) + cfg.NSeqMax() - 1) / cfg.NSeqMax())
	cfg.PtrIncrementalCache = new(false)
	cfg.Speculation = model.SpeculationDisabled
	testlib.WithModel(t, cfg, func(t *testing.T, krn *kronk.Kronk) {
		testlib.TestModelProfile(t, krn, "Dense", "Attention-only", "qwen3")
		testlib.TestChatIsolation(t, krn, words, false)
	})
}
