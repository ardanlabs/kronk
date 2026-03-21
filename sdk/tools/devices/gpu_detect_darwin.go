//go:build darwin

package devices

import (
	"runtime"

	"github.com/hybridgroup/yzma/pkg/download"
)

// DetectGPU probes the OS for available GPU hardware without depending on
// llama.cpp libraries. On macOS ARM64 (Apple Silicon), it returns Metal.
// On Intel Macs (amd64), it returns CPU because the precompiled Metal
// libraries are only available for ARM64. The x64 macOS binary already
// includes Metal support, so Intel Macs still get GPU acceleration via
// the CPU processor selection.
func DetectGPU() download.Processor {
	if runtime.GOARCH == "arm64" {
		return download.MustParseProcessor("metal")
	}

	return download.CPU
}
