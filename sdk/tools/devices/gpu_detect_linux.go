//go:build linux

package devices

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/hybridgroup/yzma/pkg/download"
)

// DetectGPU probes the OS for available GPU hardware without depending on
// llama.cpp libraries. It checks for CUDA (via nvidia-smi), ROCm (via
// rocminfo), and Vulkan (via vulkaninfo) in priority order. If no GPU is
// found, it returns download.CPU.
func DetectGPU() download.Processor {
	if hasCUDA, _ := download.HasCUDA(); hasCUDA {
		return download.MustParseProcessor("cuda")
	}

	if hasROCm, _ := download.HasROCm(); hasROCm {
		return download.MustParseProcessor("rocm")
	}

	if hasVulkan() {
		return download.MustParseProcessor("vulkan")
	}

	return download.CPU
}

// hasVulkan checks for Vulkan support on Linux by running vulkaninfo.
func hasVulkan() bool {
	if _, err := exec.LookPath("vulkaninfo"); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "vulkaninfo", "--summary").CombinedOutput()
	if err != nil {
		return false
	}

	return strings.Contains(string(out), "Vulkan Instance")
}
