package model

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestYzmaSpeechInputLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("layout expectations are for the supported 64-bit native builds")
	}
	var input MTMDHelperGenAudioInput
	if got := unsafe.Sizeof(input); got != 72 {
		t.Fatalf("speech input size: got %d, want C size 72", got)
	}
	for _, tt := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"sequence", unsafe.Offsetof(input.SeqID), 4},
		{"prompt", unsafe.Offsetof(input.Prompt), 8},
		{"length", unsafe.Offsetof(input.PromptLen), 16},
		{"past", unsafe.Offsetof(input.NPast), 24},
		{"mode", unsafe.Offsetof(input.OutMode), 28},
		{"speaker", unsafe.Offsetof(input.SpeakerRef), 32},
		{"language", unsafe.Offsetof(input.Lang), 40},
		{"top k", unsafe.Offsetof(input.TopK), 48},
		{"top p", unsafe.Offsetof(input.TopP), 52},
		{"temperature", unsafe.Offsetof(input.Temp), 56},
		{"seed", unsafe.Offsetof(input.Seed), 60},
		{"format", unsafe.Offsetof(input.OutType), 64},
	} {
		if tt.got != tt.want {
			t.Errorf("%s offset: got %d, want %d", tt.name, tt.got, tt.want)
		}
	}
}

func TestYzmaTokenizeNUL(t *testing.T) {
	libPath := libs.Path(os.Getenv("KRONK_TEST_LIB_PATH"))
	modelPath := installedTestModel(t, "KRONK_TEST_MODEL", "Qwen3-0.6B-Q8_0.gguf")

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

// installedTestModel resolves an installed fixture unless an explicit path is
// supplied. Missing fixtures fail in CI and remain optional for local runs.
func installedTestModel(t *testing.T, env, name string) string {
	t.Helper()
	if file := os.Getenv(env); file != "" {
		return file
	}
	pattern := filepath.Join(defaults.BaseDir(""), "models", "*", "*", name)
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		if os.Getenv("CI") == "true" {
			t.Fatalf("required model %s not installed", name)
		}
		t.Skipf("model %s not installed", name)
	}
	return files[0]
}
