package hardware

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Options configures how detection is performed.
type Options struct {
	LlamaBenchPath string
	Timeout        time.Duration
}

// Detect performs hardware detection and returns the results.
func Detect(ctx context.Context, opt Options) (HardwareInfo, error) {
	var info HardwareInfo
	var cpuOK, memOK, gpuOK bool

	info.CPU.Packages = cpuPackages()
	if info.CPU.Packages > 0 {
		cpuOK = true
	}

	info.CPU.LogicalCores = logicalCores()

	ram, err := totalRAMBytes()
	if err == nil && ram > 0 {
		info.CPU.TotalRAMBytes = ram
		memOK = true
	}

	gpus, err := detectGPUs(ctx, opt)
	if err == nil && len(gpus) > 0 {
		info.GPUs = gpus
		gpuOK = true
	}

	if !cpuOK && !memOK && !gpuOK {
		return info, fmt.Errorf("hardware detection failed: no CPU, memory, or GPU information available")
	}

	return info, nil
}

func detectGPUs(ctx context.Context, opt Options) ([]GPUDevice, error) {
	path := findLlamaBench(opt.LlamaBenchPath)

	if path != "" {
		gpus, err := detectGPUsViaLlamaBench(ctx, path, opt.Timeout)
		if err == nil {
			return gpus, nil
		}
	}

	return detectGPUsFallback(ctx, opt.Timeout)
}

func findLlamaBench(override string) string {
	if override != "" {
		if _, err := os.Stat(override); err == nil {
			return override
		}
	}

	if path, err := exec.LookPath("llama-bench"); err == nil {
		return path
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	path := filepath.Join(homeDir, ".kronk", "libraries", "llama-bench")
	if _, err := os.Stat(path); err == nil {
		return path
	}

	return ""
}
