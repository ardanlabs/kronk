//go:build windows

package devices

import (
	"os/exec"
	"strings"

	"github.com/hybridgroup/yzma/pkg/download"
)

// DetectGPU probes the OS for available GPU hardware without depending on
// llama.cpp libraries. It returns the best available processor type by
// checking for CUDA (via nvidia-smi) and falling back to Vulkan (via
// vulkaninfo or driver registry presence). If no GPU is found, it returns
// download.CPU.
func DetectGPU() download.Processor {
	if hasCUDA, _ := download.HasCUDA(); hasCUDA {
		return download.MustParseProcessor("cuda")
	}

	if hasVulkan() {
		return download.MustParseProcessor("vulkan")
	}

	return download.CPU
}

// hasVulkan checks for Vulkan support on Windows by looking for the
// vulkaninfo utility on PATH or the Vulkan runtime DLL in System32.
func hasVulkan() bool {
	if _, err := exec.LookPath("vulkaninfo"); err == nil {
		out, err := exec.Command("vulkaninfo", "--summary").CombinedOutput()
		if err == nil && strings.Contains(string(out), "Vulkan Instance") {
			return true
		}
	}

	// Check for the Vulkan runtime loader DLL which is present when Vulkan
	// drivers are installed.
	if _, err := exec.LookPath("vulkan-1.dll"); err == nil {
		return true
	}

	return false
}
