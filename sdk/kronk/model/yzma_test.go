package model

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

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
