// Package testlib provides shared test infrastructure for the bucky
// runtime test packages under sdk/bucky/tests. It mirrors the role
// sdk/kronk/tests/testlib plays for the llama runtime: one-shot
// initialization, model resolution against the bucky catalog, and a
// WithWhisper helper that owns the *bucky.Bucky lifecycle.
package testlib

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/bucky/model"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// Settings controls test behavior. Names and semantics mirror
// sdk/kronk/tests/testlib so contributors can move between the two
// trees without re-learning the knobs.
var (
	TestDuration  = 60 * time.Second
	Goroutines    = 2
	MaxRetries    = 3
	RunInParallel = false
	AudioFile     string
)

// Model paths resolved during Setup. Each entry corresponds to one
// short whisper model name. Tests check for an empty ModelFiles
// slice and skip cleanly when the model has not been downloaded.
var (
	MPTinyEn buckymodels.Path
)

// =============================================================================

// Setup initializes the test environment. Call it from each
// package's TestMain. Setup is safe to call multiple times — bucky.Init
// itself is idempotent — but in practice each test binary calls it
// exactly once.
func Setup() {
	gw := os.Getenv("GITHUB_WORKSPACE")
	AudioFile = filepath.Join(gw, "examples", "samples", "jfk.wav")

	if os.Getenv("GITHUB_ACTIONS") == "true" {
		Goroutines = 1
	}

	if os.Getenv("RUN_IN_PARALLEL") == "yes" {
		RunInParallel = true
	}

	fmt.Println("Initializing bucky models system...")
	mdls, err := buckymodels.New()
	if err != nil {
		fmt.Printf("creating models system: %s\n", err)
		os.Exit(1)
	}

	resolveModel(mdls, "ggml-tiny.bin", &MPTinyEn)

	printInfo(mdls)

	fmt.Println("Init Bucky...")
	if err := bucky.Init(); err != nil {
		fmt.Printf("Failed to init the whisper.cpp library: error: %s\n", err)
		os.Exit(1)
	}
}

func resolveModel(mdls *buckymodels.Models, name string, mp *buckymodels.Path) {
	if dp, err := mdls.FullPath(name); err == nil {
		fmt.Printf("RetrieveModel %s...\n", name)
		*mp = dp
	}
}

func printInfo(mdls *buckymodels.Models) {
	fmt.Println("libpath          :", buckylibs.Path(""))
	fmt.Println("modelPath        :", mdls.Path())
	fmt.Println("audioFile        :", AudioFile)
	fmt.Println("goroutines       :", Goroutines)
	fmt.Println("maxRetries       :", MaxRetries)
	fmt.Println("testDuration     :", TestDuration)
	fmt.Println("RUN_IN_PARALLEL  :", RunInParallel)
}

// =============================================================================

// WithWhisper creates a Whisper handle for the duration of fn,
// handling cleanup. Mirrors testlib.WithModel in sdk/kronk/tests.
func WithWhisper(t *testing.T, cfg model.Config, fn func(t *testing.T, w *bucky.Bucky)) {
	t.Helper()

	w, err := bucky.New(model.WithConfig(cfg))
	if err != nil {
		t.Fatalf("unable to load model %q: %v", cfg.ModelPath, err)
	}

	t.Cleanup(func() {
		t.Logf("active streams: %d", w.ActiveStreams())
		t.Log("unloading whisper")
		if err := w.Unload(context.Background()); err != nil {
			t.Errorf("failed to unload whisper: %v", err)
		}
	})

	fn(t, w)
}

// =============================================================================
// Config builders for each model variant.

// CfgTinyEn returns a model.Config for the multilingual ggml-tiny.bin
// whisper model sized for the parallel transcribe tests (NSeqMax=2).
func CfgTinyEn() model.Config {
	return model.Config{
		ModelPath: MPTinyEn.ModelFiles[0],
		UseGPU:    true,
		NSeqMax:   2,
	}
}
