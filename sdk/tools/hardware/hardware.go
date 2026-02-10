// Package hardware provides system device and memory detection.
package hardware

import (
	"fmt"
)

// GPUBackend identifies the compute backend for a GPU device.
type GPUBackend string

const (
	BackendCUDA    GPUBackend = "cuda"
	BackendMetal   GPUBackend = "metal"
	BackendROCm    GPUBackend = "rocm"
	BackendVulkan  GPUBackend = "vulkan"
	BackendUnknown GPUBackend = "unknown"
)

// GPUDevice represents a single GPU with its total VRAM.
type GPUDevice struct {
	ID        string
	Name      string
	Backend   GPUBackend
	VRAMBytes uint64
}

func (d GPUDevice) String() string {
	vramGB := float64(d.VRAMBytes) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%s: %s [%s] (%.2f GB)", d.ID, d.Name, d.Backend, vramGB)
}

// CPUInfo describes the CPU and system memory.
type CPUInfo struct {
	Packages      int
	LogicalCores  int
	TotalRAMBytes uint64
}

func (c CPUInfo) String() string {
	ramGB := float64(c.TotalRAMBytes) / (1024 * 1024 * 1024)
	return fmt.Sprintf("Packages[%d] Cores[%d] RAM[%.2f GB]", c.Packages, c.LogicalCores, ramGB)
}

// HardwareInfo is the top-level result from device detection.
type HardwareInfo struct {
	CPU  CPUInfo
	GPUs []GPUDevice
}

// GPUCount returns the number of detected GPU devices.
func (h HardwareInfo) GPUCount() int {
	return len(h.GPUs)
}

// TotalVRAMBytes returns the sum of VRAM across all detected GPUs.
func (h HardwareInfo) TotalVRAMBytes() uint64 {
	var sum uint64
	for _, g := range h.GPUs {
		sum += g.VRAMBytes
	}
	return sum
}

func (h HardwareInfo) String() string {
	vramGB := float64(h.TotalVRAMBytes()) / (1024 * 1024 * 1024)
	return fmt.Sprintf("CPU[%s] GPUs[%d] TotalVRAM[%.2f GB]", h.CPU, h.GPUCount(), vramGB)
}
