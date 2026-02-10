package hardware

import (
	"context"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLlamaBenchTimeout = 30 * time.Second
	mibToBytes               = 1024 * 1024
)

var deviceLineRe = regexp.MustCompile(`^\s*(\S+):\s+(.+?)\s+\((\d+(?:\.\d+)?)\s+MiB,\s+(\d+(?:\.\d+)?)\s+MiB\s+free\)`)

func detectGPUsViaLlamaBench(ctx context.Context, path string, timeout time.Duration) ([]GPUDevice, error) {
	if path == "" {
		return nil, fmt.Errorf("llama-bench path is empty")
	}

	if timeout == 0 {
		timeout = defaultLlamaBenchTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--list-devices")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("llama-bench --list-devices: %w", err)
	}

	return parseLlamaBenchDevices(string(out))
}

func parseLlamaBenchDevices(output string) ([]GPUDevice, error) {
	lines := strings.Split(output, "\n")

	start := 0
	for i, line := range lines {
		if strings.TrimSpace(line) == "Available devices:" {
			start = i + 1
			break
		}
	}

	var devices []GPUDevice
	for _, line := range lines[start:] {
		matches := deviceLineRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		id := matches[1]
		name := matches[2]

		totalMiB, err := strconv.ParseFloat(matches[3], 64)
		if err != nil {
			continue
		}

		if totalMiB == 0 {
			continue
		}

		devices = append(devices, GPUDevice{
			ID:        id,
			Name:      name,
			Backend:   inferBackend(id),
			VRAMBytes: uint64(math.Round(totalMiB)) * mibToBytes,
		})
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices found in llama-bench output")
	}

	return devices, nil
}

func inferBackend(id string) GPUBackend {
	upper := strings.ToUpper(id)

	switch {
	case strings.HasPrefix(upper, "MTL"):
		return BackendMetal
	case strings.HasPrefix(upper, "CUDA"):
		return BackendCUDA
	case strings.HasPrefix(upper, "ROCM"):
		return BackendROCm
	case strings.HasPrefix(upper, "VK"):
		return BackendVulkan
	default:
		return BackendUnknown
	}
}
