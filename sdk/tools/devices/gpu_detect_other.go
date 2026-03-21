//go:build !windows && !darwin && !linux

package devices

import "github.com/hybridgroup/yzma/pkg/download"

// DetectGPU is a placeholder for unsupported platforms. It returns
// download.CPU until platform-specific detection is implemented.
func DetectGPU() download.Processor {
	return download.CPU
}
