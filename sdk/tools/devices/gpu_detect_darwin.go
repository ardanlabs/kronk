//go:build darwin

package devices

import "github.com/hybridgroup/yzma/pkg/download"

// DetectGPU returns Metal on macOS. All supported Macs have Metal-capable GPUs.
func DetectGPU() download.Processor {
	return download.MustParseProcessor("metal")
}
