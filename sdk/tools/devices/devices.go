// Package devices provides compute device enumeration and system memory detection.
package devices

import (
	"strings"
	"sync/atomic"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// llamaReady reports whether the llama.cpp FFI bindings have been fully
// loaded. It is false until llama.Load succeeds. The device enumeration
// functions in this package call into those bindings, which panic on a nil
// FFI binding if invoked before a successful load (e.g. when the server runs
// in degraded mode because llama.cpp could not be installed).
//
// This flag lives in the devices package because it is a low-level leaf
// (importing only yzma/llama). Both sdk/kronk (which sets it) and sdk/bucky
// (which reads it) can depend on it without creating an import cycle, whereas
// they could not share a flag owned by sdk/kronk.
var llamaReady atomic.Bool

// SetReady records whether the llama.cpp FFI bindings are loaded. sdk/kronk
// calls this with true immediately after llama.Load succeeds.
func SetReady(v bool) {
	llamaReady.Store(v)
}

// Ready reports whether the llama.cpp FFI bindings are loaded and therefore
// safe to call. Callers that would otherwise invoke llama FFI functions must
// check this first to avoid a nil-binding panic in degraded mode.
func Ready() bool {
	return llamaReady.Load()
}

// DeviceInfo provides information about a single compute device.
type DeviceInfo struct {
	Index      int    `json:"index"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	FreeBytes  uint64 `json:"free_bytes"`
	TotalBytes uint64 `json:"total_bytes"`
}

// Devices returns information about available compute devices.
type Devices struct {
	Devices            []DeviceInfo `json:"devices"`
	GPUCount           int          `json:"gpu_count"`
	GPUTotalBytes      uint64       `json:"gpu_total_bytes"`
	SupportsGPUOffload bool         `json:"supports_gpu_offload"`
	MaxDevices         uint64       `json:"max_devices"`
	SystemRAMBytes     uint64       `json:"system_ram_bytes"`
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
	// In degraded mode the llama.cpp FFI bindings are not loaded, so any call
	// into them (GGMLBackendDeviceCount, SupportsGpuOffload, MaxDevices, ...)
	// would dereference a nil binding and panic. Report zero devices instead.
	if !Ready() {
		return Devices{}
	}

	cfg := defaultOptions()
	for _, o := range opts {
		o(&cfg)
	}

	count := llama.GGMLBackendDeviceCount()

	var out Devices
	out.Devices = make([]DeviceInfo, 0, count)

	for i := range count {
		dev := llama.GGMLBackendDeviceGet(i)
		if dev == 0 {
			continue
		}

		name := llama.GGMLBackendDeviceName(dev)
		devType := ClassifyDeviceType(name)

		if !cfg.includeCPU && devType == "cpu" {
			continue
		}
		if !cfg.includeUnknown && devType == "unknown" {
			continue
		}

		di := DeviceInfo{
			Index: int(i),
			Name:  name,
			Type:  devType,
		}

		if cfg.includeMemory {
			di.FreeBytes, di.TotalBytes = llama.GGMLBackendDeviceMemory(dev)
		}

		out.Devices = append(out.Devices, di)

		if strings.HasPrefix(devType, "gpu_") {
			out.GPUCount++
			out.GPUTotalBytes += di.TotalBytes
		}
	}

	out.SupportsGPUOffload = llama.SupportsGpuOffload()
	out.MaxDevices = llama.MaxDevices()

	if cfg.includeMemory {
		out.SystemRAMBytes = SystemRAMBytes()
	}

	return out
}

// ClassifyDeviceType maps a llama.cpp backend device name to a device type
// string: cpu, gpu_cuda, gpu_metal, gpu_rocm, gpu_vulkan, or unknown.
func ClassifyDeviceType(name string) string {
	switch {
	case name == "CPU":
		return "cpu"
	case strings.HasPrefix(name, "CUDA"):
		return "gpu_cuda"
	case name == "Metal":
		return "gpu_metal"
	case strings.HasPrefix(name, "HIP"), strings.HasPrefix(name, "ROCm"):
		return "gpu_rocm"
	case strings.HasPrefix(name, "Vulkan"):
		return "gpu_vulkan"
	default:
		return "unknown"
	}
}
