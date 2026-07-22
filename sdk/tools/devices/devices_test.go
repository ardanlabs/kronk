package devices

import "testing"

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

	got = List(WithIncludeMemory(false))
	if got.SystemRAMBytes != 0 {
		t.Errorf("SystemRAMBytes without memory: got %d, want 0", got.SystemRAMBytes)
	}
}

func TestClassifyDeviceType(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"CPU", "cpu"},
		{"CUDA0", "gpu_cuda"},
		{"CUDA1", "gpu_cuda"},
		{"Metal", "gpu_metal"},
		{"HIP0", "gpu_rocm"},
		{"ROCm0", "gpu_rocm"},
		{"ROCm1", "gpu_rocm"},
		{"Vulkan0", "gpu_vulkan"},
		{"SomethingElse", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyDeviceType(tt.name); got != tt.want {
				t.Errorf("ClassifyDeviceType(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}
