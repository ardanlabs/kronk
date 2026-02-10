//go:build linux

package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultGPUFallbackTimeout = 10 * time.Second

func detectGPUsFallback(ctx context.Context, timeout time.Duration) ([]GPUDevice, error) {
	if timeout == 0 {
		timeout = defaultGPUFallbackTimeout
	}

	if devices := detectGPUsViaNvidiaSmi(ctx, timeout); len(devices) > 0 {
		return devices, nil
	}

	if devices := detectGPUsViaDRMSysfs(); len(devices) > 0 {
		return devices, nil
	}

	if devices := detectGPUsViaROCmSmi(ctx, timeout); len(devices) > 0 {
		return devices, nil
	}

	return nil, nil
}

// =============================================================================
// NVIDIA

func detectGPUsViaNvidiaSmi(ctx context.Context, timeout time.Duration) []GPUDevice {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path,
		"--query-gpu=index,name,memory.total",
		"--format=csv,noheader,nounits",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	devices, _ := parseNvidiaSmi(string(out))

	return devices
}

func parseNvidiaSmi(output string) ([]GPUDevice, error) {
	var devices []GPUDevice

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ",", 3)
		if len(parts) != 3 {
			continue
		}

		idx := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		memStr := strings.TrimSpace(parts[2])

		memMiB, err := strconv.ParseUint(memStr, 10, 64)
		if err != nil {
			continue
		}

		devices = append(devices, GPUDevice{
			ID:        idx,
			Name:      name,
			Backend:   BackendCUDA,
			VRAMBytes: memMiB * mibToBytes,
		})
	}

	return devices, nil
}

// =============================================================================
// AMD DRM sysfs — works without any userspace tools installed.
//
// Each AMD GPU exposes VRAM info at:
//   /sys/class/drm/card<N>/device/mem_info_vram_total  (bytes)
//
// The product name can be read from:
//   /sys/class/drm/card<N>/device/product_name  (server cards)
//   /sys/class/drm/card<N>/device/vendor + device  (PCI IDs fallback)

func detectGPUsViaDRMSysfs() []GPUDevice {
	matches, err := filepath.Glob("/sys/class/drm/card[0-9]*/device/mem_info_vram_total")
	if err != nil || len(matches) == 0 {
		return nil
	}

	var devices []GPUDevice
	for _, vramPath := range matches {
		data, err := os.ReadFile(vramPath)
		if err != nil {
			continue
		}

		vramBytes, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
		if err != nil || vramBytes == 0 {
			continue
		}

		deviceDir := filepath.Dir(vramPath)
		cardDir := filepath.Dir(deviceDir)
		cardName := filepath.Base(cardDir)

		name := readSysfsString(filepath.Join(deviceDir, "product_name"))
		if name == "" {
			name = "AMD GPU"
		}

		devices = append(devices, GPUDevice{
			ID:        cardName,
			Name:      name,
			Backend:   BackendROCm,
			VRAMBytes: vramBytes,
		})
	}

	return devices
}

func readSysfsString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// =============================================================================
// ROCm SMI — fallback for AMD GPUs when DRM sysfs is unavailable.
//
// Tries rocm-smi first, then amd-smi (the newer replacement).
// Uses --showmeminfo vram --json which outputs per-GPU VRAM totals.

func detectGPUsViaROCmSmi(ctx context.Context, timeout time.Duration) []GPUDevice {
	if devices := tryROCmSmi(ctx, timeout); len(devices) > 0 {
		return devices
	}

	return tryAMDSmi(ctx, timeout)
}

func tryROCmSmi(ctx context.Context, timeout time.Duration) []GPUDevice {
	path, err := exec.LookPath("rocm-smi")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--showmeminfo", "vram", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseROCmSmiJSON(out)
}

func parseROCmSmiJSON(data []byte) []GPUDevice {
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}

	var devices []GPUDevice

	for key, val := range result {
		if !strings.HasPrefix(key, "card") {
			continue
		}

		cardMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		var vramBytes uint64
		if total, ok := cardMap["VRAM Total Memory (B)"].(float64); ok && total > 0 {
			vramBytes = uint64(total)
		} else if totalStr, ok := cardMap["VRAM Total Memory (B)"].(string); ok {
			vramBytes, _ = strconv.ParseUint(totalStr, 10, 64)
		}

		if vramBytes == 0 {
			continue
		}

		devices = append(devices, GPUDevice{
			ID:        key,
			Name:      "AMD GPU",
			Backend:   BackendROCm,
			VRAMBytes: vramBytes,
		})
	}

	return devices
}

func tryAMDSmi(ctx context.Context, timeout time.Duration) []GPUDevice {
	path, err := exec.LookPath("amd-smi")
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "static", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseAMDSmiJSON(out)
}

func parseAMDSmiJSON(data []byte) []GPUDevice {
	var gpus []map[string]any
	if err := json.Unmarshal(data, &gpus); err != nil {
		return nil
	}

	var devices []GPUDevice

	for i, gpu := range gpus {
		vramMap, ok := gpu["vram"].(map[string]any)
		if !ok {
			continue
		}

		sizeMap, ok := vramMap["size"].(map[string]any)
		if !ok {
			continue
		}

		valueMB, ok := sizeMap["value"].(float64)
		if !ok || valueMB == 0 {
			continue
		}

		devices = append(devices, GPUDevice{
			ID:        fmt.Sprintf("%d", i),
			Name:      "AMD GPU",
			Backend:   BackendROCm,
			VRAMBytes: uint64(valueMB) * mibToBytes,
		})
	}

	return devices
}
