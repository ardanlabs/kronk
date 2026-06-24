package devices

import "testing"

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
