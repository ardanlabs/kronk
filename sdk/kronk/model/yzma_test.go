package model

import (
	"os"
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestYzmaMTMDTextCapabilities(t *testing.T) {
	libPath := os.Getenv("KRONK_TEST_LIB_PATH")
	if libPath == "" {
		t.Skip("KRONK_TEST_LIB_PATH is required (llama.cpp b11899 or newer)")
	}

	if err := InitYzmaWorkarounds(libPath); err != nil {
		t.Fatalf("initialize mtmd bindings: %v", err)
	}

	// With no projector, native capability discovery returns text-only defaults
	// without accessing a model. No model weights or GPU initialization are needed.
	got, err := MTMDHelperGetCapabilities(0, 0)
	if err != nil {
		t.Fatalf("get text-only capabilities: %v", err)
	}

	want := MTMDHelperCapabilities{Chat: true, TextGen: true}
	if got != want {
		t.Errorf("capabilities: got %+v, want %+v", got, want)
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
