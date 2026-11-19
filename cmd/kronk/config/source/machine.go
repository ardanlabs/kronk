package source

import (
	"fmt"
	"slices"

	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// FormatBytes shows a size in GB, or MB below 1 GB.
func FormatBytes(n uint64) string {
	const gb = 1 << 30
	const mb = 1 << 20
	if n >= gb {
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	}
	return fmt.Sprintf("%d MB", n/mb)
}

// gpuType is a GPU backend a target machine can have. name is the llama.cpp
// device name prefix, used to build CUDA0, CUDA1, ...
type gpuType struct {
	Label, Value, name string
}

// GPUTypes are the GPU backends a target machine can have.
var GPUTypes = []gpuType{
	{"NVIDIA (CUDA)", "cuda", "CUDA"},
	{"AMD (ROCm)", "rocm", "ROCm"},
	{"Vulkan", "vulkan", "Vulkan"},
	{"Apple Silicon (Metal)", "metal", "MTL"},
}

// Machine is a target machine described by the user.
type Machine struct {
	GPUs    int
	GPUType string
	VRAM    uint64 // per GPU
	RAM     uint64
}

// Devices builds the device list the analysis reads, as llama.cpp would
// report it on that machine.
func (m Machine) Devices() devices.Devices {
	devs := devices.Devices{
		Devices:        []devices.DeviceInfo{{Name: "CPU", Type: "cpu", HardwareType: "cpu"}},
		SystemRAMBytes: m.RAM,
		MMapSupported:  true,
		UnifiedMemory:  m.GPUType == "metal",
	}

	i := slices.IndexFunc(GPUTypes, func(t gpuType) bool { return t.Value == m.GPUType })
	for n := range m.GPUs {
		name := fmt.Sprintf("GPU%d", n)
		if i >= 0 {
			name = fmt.Sprintf("%s%d", GPUTypes[i].name, n)
		}
		devs.Devices = append(devs.Devices, devices.DeviceInfo{
			Index:        n + 1,
			Name:         name,
			Type:         "gpu_" + m.GPUType,
			HardwareType: "gpu",
			Backend:      m.GPUType,
			FreeBytes:    m.VRAM,
			TotalBytes:   m.VRAM,
		})
		devs.GPUCount++
		devs.GPUTotalBytes += m.VRAM
		devs.SupportsGPUOffload = true
	}

	return devs
}

// String describes the machine for the screens.
func (m Machine) String() string {
	if m.GPUs == 0 {
		return fmt.Sprintf("another machine: CPU only, %s RAM", FormatBytes(m.RAM))
	}

	name := m.GPUType
	for _, t := range GPUTypes {
		if t.Value == m.GPUType {
			name = t.Label
		}
	}
	return fmt.Sprintf("another machine: %d × %s, %s each, %s RAM", m.GPUs, name, FormatBytes(m.VRAM), FormatBytes(m.RAM))
}
