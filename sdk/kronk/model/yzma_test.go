package model

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

func TestYzmaCapabilityABI(t *testing.T) {
	// Exercise actual C struct returns, not a Go mock of libffi. Each field is
	// enabled alone so a shifted output flag cannot accidentally pass.
	oldDecision, oldCap, oldDuplex := batchExtSetDecisionGroupFunc, helperGetCapabilitiesFunc, helperCapDuplex
	t.Cleanup(func() {
		batchExtSetDecisionGroupFunc, helperGetCapabilitiesFunc, helperCapDuplex = oldDecision, oldCap, oldDuplex
	})
	for _, tt := range []struct {
		name    string
		version string
		duplex  bool
		missing bool
	}{
		{name: "before duplex", version: "b12618"},
		{name: "first duplex", version: "b12619", duplex: true},
		{name: "pinned version", version: "b12651@sha256:test", duplex: true},
		{name: "before capabilities", missing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := buildCapabilityLibrary(t, tt.duplex, tt.missing)
			if err := os.WriteFile(filepath.Join(dir, "version.json"), fmt.Appendf(nil, `{"version":%q}`, tt.version), 0600); err != nil {
				t.Fatal(err)
			}
			if err := InitYzmaWorkarounds(dir); err != nil {
				t.Fatal(err)
			}
			if tt.missing {
				if _, err := helperGetCapabilities(1, 0); !errors.Is(err, mtmd.ErrNoCapabilities) {
					t.Fatalf("got error %v, want ErrNoCapabilities", err)
				}
				return
			}
			want := []mtmd.HelperCap{
				{Chat: true}, {Vision: true}, {Audio: true}, {Video: true}, {AudioStream: true},
				{TextGen: true}, {AudioGen: true}, {AudioGenSpkRef: true},
			}
			if tt.duplex {
				want = slices.Insert(want, 5, mtmd.HelperCap{})
			}
			for i, expected := range want {
				got, err := helperGetCapabilities(1, mtmd.Context(1<<i))
				if err != nil || got != expected {
					t.Errorf("field %d: got %+v, %v; want %+v, nil", i, got, err, expected)
				}
			}
			if got, err := helperGetCapabilities(0, 255); err != nil || got != (mtmd.HelperCap{}) {
				t.Errorf("null model: got %+v, %v; want empty capabilities, nil", got, err)
			}
			for _, version := range []string{"", "custom", "12619", "b-1"} {
				if err := os.WriteFile(filepath.Join(dir, "version.json"), fmt.Appendf(nil, `{"version":%q}`, version), 0600); err != nil {
					t.Fatal(err)
				}
				if err := InitYzmaWorkarounds(dir); err == nil {
					t.Errorf("version %q: got nil error, want unknown ABI error", version)
				}
			}
			if err := os.Remove(filepath.Join(dir, "version.json")); err != nil {
				t.Fatal(err)
			}
			if err := InitYzmaWorkarounds(dir); err == nil {
				t.Error("missing version.json: got nil error, want unknown ABI error")
			}
		})
	}
}

func buildCapabilityLibrary(t *testing.T, duplex, missing bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("C fixture currently requires a Unix C compiler")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler not installed")
	}
	dir := t.TempDir()
	source := `
#include <stdbool.h>
#include <stdint.h>
struct cap {
    bool chat, vision, audio, video, audio_stream;
#if DUPLEX
    bool audio_duplex;
#endif
    bool text_gen, audio_gen, audio_gen_spk_ref;
};
#if !MISSING
struct cap mtmd_helper_get_capabilities(const void *model, const void *ctx) {
    uintptr_t bits = (uintptr_t)ctx;
    struct cap result = {false};
    bool *fields = (bool *)&result;
    for (unsigned i = 0; i < sizeof(result); i++) fields[i] = (bits >> i) & 1;
    return result;
}
#endif
void placeholder(void) {}
`
	file := filepath.Join(dir, "cap.c")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	flag := "-shared"
	if runtime.GOOS == "darwin" {
		flag = "-dynamiclib"
	}
	for _, lib := range []string{"llama", "mtmd"} {
		args := []string{flag, "-fPIC", file, "-o", loader.GetLibraryFilename(dir, lib)}
		if duplex {
			args = append(args, "-DDUPLEX=1")
		}
		if missing {
			args = append(args, "-DMISSING=1")
		}
		if out, err := exec.CommandContext(t.Context(), cc, args...).CombinedOutput(); err != nil {
			t.Fatalf("build C fixture: %v\n%s", err, out)
		}
	}
	return dir
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
