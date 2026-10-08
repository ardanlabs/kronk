package model

import (
	"os"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestYzmaMoECacheContextParams(t *testing.T) {
	path := os.Getenv("KRONK_TEST_LIB_PATH")
	if path == "" {
		t.Skip("KRONK_TEST_LIB_PATH is required")
	}
	if err := llama.Load(path); err != nil {
		t.Fatal(err)
	}
	if err := llama.Init(); err != nil {
		t.Fatal(err)
	}
	for _, bytes := range []int64{0, 123456789} {
		cfg := NewConfig(WithMoE(&MoEConfig{PtrCacheSize: new(bytes)}))
		got := modelCtxParams(cfg, ModelInfo{})
		if got.MoeCacheSize != uint64(bytes) {
			t.Fatalf("native cache bytes=%d, want %d", got.MoeCacheSize, bytes)
		}
	}
}

func TestYzmaTokenizeNUL(t *testing.T) {
	libPath := os.Getenv("KRONK_TEST_LIB_PATH")
	modelPath := os.Getenv("KRONK_TEST_MODEL")
	if libPath == "" || modelPath == "" {
		t.Skip("KRONK_TEST_LIB_PATH and KRONK_TEST_MODEL are required")
	}

	if err := llama.Load(libPath); err != nil {
		t.Fatalf("load llama.cpp: %v", err)
	}
	if err := llama.Init(); err != nil {
		t.Fatalf("initialize llama.cpp: %v", err)
	}

	params := llama.ModelDefaultParams()
	params.VocabOnly = 1
	mdl, err := llama.ModelLoadFromFile(modelPath, params)
	if err != nil {
		t.Fatalf("load model vocabulary: %v", err)
	}
	t.Cleanup(func() {
		if err := llama.ModelFree(mdl); err != nil {
			t.Errorf("free model: %v", err)
		}
	})

	vocab := llama.ModelGetVocab(mdl)
	tokens := llama.Tokenize(vocab, "left\x00right", false, true)
	if len(tokens) == 0 {
		t.Fatal("tokenize embedded NUL: got no tokens")
	}
	if prefix := llama.Tokenize(vocab, "left", false, true); slices.Equal(tokens, prefix) {
		t.Fatalf("tokenize embedded NUL: got prefix tokens %v; bytes after NUL had no effect", tokens)
	}
}
