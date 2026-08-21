// Package devices provides compute device enumeration and system memory detection.
package devices

import (
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// llamaReady reports whether the llama.cpp FFI bindings are loaded.
var llamaReady atomic.Bool

// SetReady records whether the llama.cpp FFI bindings are loaded.
func SetReady(ready bool) {
	llamaReady.Store(ready)
}

// Ready reports whether the llama.cpp FFI bindings are safe to call.
func Ready() bool {
	return llamaReady.Load()
}

// DeviceInfo provides information about a single compute device.
type DeviceInfo struct {
	Index        int    `json:"index"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Type         string `json:"type"`
	HardwareType string `json:"hardware_type"`
	Backend      string `json:"backend"`
	FreeBytes    uint64 `json:"free_bytes"`
	TotalBytes   uint64 `json:"total_bytes"`
}

// Devices returns information about available compute devices.
type Devices struct {
	Devices            []DeviceInfo `json:"devices"`
	GPUCount           int          `json:"gpu_count"`
	GPUTotalBytes      uint64       `json:"gpu_total_bytes"`
	SupportsGPUOffload bool         `json:"supports_gpu_offload"`
	MaxDevices         uint64       `json:"max_devices"`
	SystemRAMBytes     uint64       `json:"system_ram_bytes"`
	UnifiedMemory      bool         `json:"unified_memory"`
}

// Option controls device enumeration behavior.
type Option func(*options)

type options struct {
	includeCPU     bool
	includeUnknown bool
	includeMemory  bool
}

func defaultOptions() options {
	return options{
		includeCPU:     true,
		includeUnknown: true,
		includeMemory:  true,
	}
}

// WithIncludeCPU controls whether CPU devices are included in the results.
func WithIncludeCPU(v bool) Option {
	return func(o *options) {
		o.includeCPU = v
	}
}

// WithIncludeUnknown controls whether unknown device types are included.
func WithIncludeUnknown(v bool) Option {
	return func(o *options) {
		o.includeUnknown = v
	}
}

// WithIncludeMemory controls whether memory stats are queried for each device.
func WithIncludeMemory(v bool) Option {
	return func(o *options) {
		o.includeMemory = v
	}
}

// List enumerates all available compute devices via the llama.cpp backend.
func List(opts ...Option) Devices {
	cfg := defaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	// llama.cpp functions dereference unresolved FFI bindings when Load has
	// not succeeded. Preserve the system RAM information that does not depend
	// on llama.cpp while reporting no backend devices in degraded mode.
	if !Ready() {
		out := Devices{
			UnifiedMemory: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",
		}
		if cfg.includeMemory {
			out.SystemRAMBytes = SystemRAMBytes()
		}
		return out
	}

	count := llama.GGMLBackendDeviceCount()

	out := Devices{
		UnifiedMemory: runtime.GOOS == "darwin" && runtime.GOARCH == "arm64",

		Devices: make([]DeviceInfo, 0, count)}

	for i := range count {
		dev := llama.GGMLBackendDeviceGet(i)
		if dev == 0 {
			continue
		}

		name := llama.GGMLBackendDeviceName(dev)
		hardwareType := llama.GGMLBackendDevType(dev)
		backend := llama.GGMLBackendRegName(llama.GGMLBackendDeviceBackendReg(dev))
		devType := classifyDeviceType(hardwareType, backend)

		if !cfg.includeCPU && devType == "cpu" {
			continue
		}
		if !cfg.includeUnknown && devType == "unknown" {
			continue
		}

		di := DeviceInfo{
			Index:        int(i),
			Name:         name,
			Description:  llama.GGMLBackendDeviceDescription(dev),
			Type:         devType,
			HardwareType: hardwareType.String(),
			Backend:      backend,
		}

		if cfg.includeMemory {
			di.FreeBytes, di.TotalBytes = llama.GGMLBackendDeviceMemory(dev)
		}

		out.Devices = append(out.Devices, di)

		if strings.HasPrefix(devType, "gpu_") {
			out.GPUCount++
			out.GPUTotalBytes += di.TotalBytes
		}
		if devType == "gpu_metal" {
			out.UnifiedMemory = true
		}
	}

	out.SupportsGPUOffload = llama.SupportsGpuOffload()
	out.MaxDevices = llama.MaxDevices()

	if cfg.includeMemory {
		out.SystemRAMBytes = SystemRAMBytes()
	}

	return out
}

// ClassifyDeviceType maps a llama.cpp backend device to a device type string:
// cpu, gpu_cuda, gpu_metal, gpu_rocm, gpu_vulkan, or unknown.
func ClassifyDeviceType(device llama.GGMLBackendDevice) string {
	if device == 0 {
		return "unknown"
	}

	deviceType := llama.GGMLBackendDevType(device)
	reg := llama.GGMLBackendDeviceBackendReg(device)

	return classifyDeviceType(deviceType, llama.GGMLBackendRegName(reg))
}

func classifyDeviceType(deviceType llama.GGMLBackendDeviceType, backend string) string {
	if deviceType == llama.GGMLBackendDeviceTypeCPU {
		return "cpu"
	}

	if deviceType != llama.GGMLBackendDeviceTypeGPU && deviceType != llama.GGMLBackendDeviceTypeIGPU {
		return "unknown"
	}

	switch strings.ToUpper(backend) {
	case "CUDA":
		return "gpu_cuda"
	case "MTL", "METAL":
		return "gpu_metal"
	case "HIP", "ROCM":
		return "gpu_rocm"
	case "VULKAN":
		return "gpu_vulkan"
	default:
		return "unknown"
	}
}
