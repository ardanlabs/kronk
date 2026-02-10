//go:build darwin

package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const defaultSystemProfilerTimeout = 30 * time.Second

func detectGPUsFallback(ctx context.Context, timeout time.Duration) ([]GPUDevice, error) {
	if timeout == 0 {
		timeout = defaultSystemProfilerTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "system_profiler", "SPDisplaysDataType", "-json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("system_profiler: %w", err)
	}

	return parseSystemProfiler(out)
}

type spDisplayData struct {
	SPDisplays []spDisplay `json:"SPDisplaysDataType"`
}

type spDisplay struct {
	Name string `json:"sppci_model"`
	VRAM string `json:"sppci_vram"`
}

func parseSystemProfiler(data []byte) ([]GPUDevice, error) {
	var sp spDisplayData
	if err := json.Unmarshal(data, &sp); err != nil {
		return nil, fmt.Errorf("parse system_profiler json: %w", err)
	}

	var devices []GPUDevice
	for i, display := range sp.SPDisplays {
		name := display.Name
		if name == "" {
			name = "Unknown GPU"
		}

		devices = append(devices, GPUDevice{
			ID:        fmt.Sprintf("GPU%d", i),
			Name:      name,
			Backend:   BackendMetal,
			VRAMBytes: parseVRAMString(display.VRAM),
		})
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no displays found in system_profiler output")
	}

	return devices, nil
}

func parseVRAMString(vram string) uint64 {
	if vram == "" {
		return 0
	}

	vram = strings.TrimSpace(vram)

	var value uint64
	var unit string
	if _, err := fmt.Sscanf(vram, "%d %s", &value, &unit); err != nil {
		return 0
	}

	switch strings.ToUpper(unit) {
	case "MB":
		return value * 1024 * 1024
	case "GB":
		return value * 1024 * 1024 * 1024
	default:
		return 0
	}
}
