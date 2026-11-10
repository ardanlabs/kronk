//go:build (linux && amd64) || (darwin && arm64) || (windows && amd64)

package pool_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/malina"
)

func TestNativeBundles(t *testing.T) {
	if testing.Short() {
		t.Skip("native library loading requires installed bundles")
	}

	// Match server startup: llama owns the shared GGML layer. Keep all
	// three bundles resident in this process; no model files are needed.
	if err := kronk.Init(); err != nil {
		t.Fatalf("load llama.cpp: %v", err)
	}
	if err := bucky.Init(); err != nil {
		t.Fatalf("load whisper.cpp after llama.cpp: %v", err)
	}
	if err := malina.Init(); err != nil {
		t.Fatalf("load stable-diffusion.cpp after llama.cpp and whisper.cpp: %v", err)
	}

	// Repeating startup must remain safe with all three bundles loaded.
	if err := kronk.Init(); err != nil {
		t.Fatalf("repeat llama.cpp initialization: %v", err)
	}
	if err := bucky.Init(); err != nil {
		t.Fatalf("repeat whisper.cpp initialization: %v", err)
	}
	if err := malina.Init(); err != nil {
		t.Fatalf("repeat stable-diffusion.cpp initialization: %v", err)
	}
}
