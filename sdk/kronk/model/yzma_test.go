package model

import (
	"os"
	"slices"
	"testing"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestTokenizeNULBuffer(t *testing.T) {
	text := "left\x00right"
	buf, ptr, size := tokenizeText(text)

	got := string(unsafe.Slice(ptr, size))
	if got != text {
		t.Fatalf("text buffer: got %q, want %q", got, text)
	}
	if size != int32(len(text)) {
		t.Fatalf("text length: got %d, want %d", size, len(text))
	}
	if buf[size] != 0 {
		t.Fatalf("text buffer terminator: got %d, want 0", buf[size])
	}
}

func TestTokenizeNULNative(t *testing.T) {
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
	if err := InitYzmaWorkarounds(libPath); err != nil {
		t.Fatalf("initialize yzma workarounds: %v", err)
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
	tokens := tokenize(vocab, "left\x00right", false, true)
	if len(tokens) == 0 {
		t.Fatal("tokenize embedded NUL: got no tokens")
	}
	if prefix := tokenize(vocab, "left", false, true); slices.Equal(tokens, prefix) {
		t.Fatalf("tokenize embedded NUL: got prefix tokens %v; bytes after NUL had no effect", tokens)
	}
}
