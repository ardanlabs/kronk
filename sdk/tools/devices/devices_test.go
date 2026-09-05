package devices

import (
	"runtime"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestListNotReady(t *testing.T) {
	wasReady := Ready()
	SetReady(false)
	t.Cleanup(func() {
		SetReady(wasReady)
	})

	got := List()
	if len(got.Devices) != 0 {
		t.Fatalf("Devices: got %d, want 0", len(got.Devices))
	}
	if got.SystemRAMBytes != SystemRAMBytes() {
		t.Errorf("SystemRAMBytes: got %d, want %d", got.SystemRAMBytes, SystemRAMBytes())
	}
	wantUnifiedMemory := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	if got.UnifiedMemory != wantUnifiedMemory {
		t.Errorf("UnifiedMemory: got %t, want %t", got.UnifiedMemory, wantUnifiedMemory)
	}

	got = List(WithIncludeMemory(false))
	if got.SystemRAMBytes != 0 {
		t.Errorf("SystemRAMBytes without memory: got %d, want 0", got.SystemRAMBytes)
	}
}

func TestBackendNotReady(t *testing.T) {
	wasReady := Ready()
	SetReady(false)
	t.Cleanup(func() {
		SetReady(wasReady)
	})

	if got := Backend(); got != "" {
		t.Errorf("Backend(): got %q, want \"\" (nothing has been enumerated to ask)", got)
	}
}

func TestBackendFromDevices(t *testing.T) {
	tests := []struct {
		name string
		devs []DeviceInfo
		want string
	}{
		{"no devices", nil, "cpu"},
		{"cpu only", []DeviceInfo{{Type: "cpu"}}, "cpu"},
		{"metal", []DeviceInfo{{Type: "gpu_metal"}, {Type: "cpu"}}, "metal"},
		// What an Apple Silicon Mac actually enumerates: the BLAS
		// accelerator classifies as "unknown" and must not win.
		{"metal behind blas", []DeviceInfo{{Type: "gpu_metal"}, {Type: "unknown"}, {Type: "cpu"}}, "metal"},
		// ggml may list the CPU device first; the GPU still decides.
		{"gpu after cpu", []DeviceInfo{{Type: "cpu"}, {Type: "gpu_vulkan"}}, "vulkan"},
		{"rocm", []DeviceInfo{{Type: "gpu_rocm"}}, "rocm"},
		{"cuda", []DeviceInfo{{Type: "gpu_cuda"}}, "cuda"},
		// A GPU ggml could not classify is not a backend name; the honest
		// answer is that no usable GPU backend was found.
		{"unclassified gpu", []DeviceInfo{{Type: "unknown"}, {Type: "cpu"}}, "cpu"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backendFromDevices(tt.devs); got != tt.want {
				t.Errorf("backendFromDevices() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyDeviceType(t *testing.T) {
	if got := ClassifyDeviceType(0); got != "unknown" {
		t.Errorf("ClassifyDeviceType(0) = %q, want %q", got, "unknown")
	}

	tests := []struct {
		name       string
		deviceType llama.GGMLBackendDeviceType
		backend    string
		want       string
	}{
		{"CPU", llama.GGMLBackendDeviceTypeCPU, "CPU", "cpu"},
		{"CUDA GPU", llama.GGMLBackendDeviceTypeGPU, "CUDA", "gpu_cuda"},
		{"CUDA integrated GPU", llama.GGMLBackendDeviceTypeIGPU, "CUDA", "gpu_cuda"},
		{"Metal", llama.GGMLBackendDeviceTypeGPU, "MTL", "gpu_metal"},
		{"Metal legacy registry", llama.GGMLBackendDeviceTypeGPU, "Metal", "gpu_metal"},
		{"HIP", llama.GGMLBackendDeviceTypeGPU, "HIP", "gpu_rocm"},
		{"ROCm", llama.GGMLBackendDeviceTypeGPU, "ROCm", "gpu_rocm"},
		{"Vulkan", llama.GGMLBackendDeviceTypeGPU, "Vulkan", "gpu_vulkan"},
		{"unknown GPU", llama.GGMLBackendDeviceTypeGPU, "SomethingElse", "unknown"},
		{"accelerator", llama.GGMLBackendDeviceTypeACCEL, "BLAS", "unknown"},
		{"meta", llama.GGMLBackendDeviceTypeMETA, "RPC", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyDeviceType(tt.deviceType, tt.backend); got != tt.want {
				t.Errorf("classifyDeviceType(%s, %q) = %q, want %q", tt.deviceType, tt.backend, got, tt.want)
			}
		})
	}
}

func TestUsesUnifiedMemory(t *testing.T) {
	tests := []struct {
		name           string
		deviceType     llama.GGMLBackendDeviceType
		classifiedType string
		want           bool
	}{
		{"Vulkan integrated GPU", llama.GGMLBackendDeviceTypeIGPU, "gpu_vulkan", true},
		{"ROCm integrated GPU", llama.GGMLBackendDeviceTypeIGPU, "gpu_rocm", true},
		{"Metal GPU", llama.GGMLBackendDeviceTypeGPU, "gpu_metal", true},
		{"Vulkan discrete GPU", llama.GGMLBackendDeviceTypeGPU, "gpu_vulkan", false},
		{"CPU", llama.GGMLBackendDeviceTypeCPU, "cpu", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := usesUnifiedMemory(tt.deviceType, tt.classifiedType); got != tt.want {
				t.Errorf("usesUnifiedMemory(%s, %q) = %t, want %t", tt.deviceType, tt.classifiedType, got, tt.want)
			}
		})
	}
}
