package hardware

import (
	"testing"
)

func TestParseLlamaBenchDevices(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		wantCount int
		wantFirst GPUDevice
		wantErr   bool
	}{
		{
			name: "macOS Metal with BLAS filtering",
			output: `ggml_metal_device_init: tensor API disabled for pre-M5 and pre-A19 devices
ggml_metal_device_init: using embedded metal library
ggml_metal_library_init: loaded in 5.253 sec
ggml_metal_rsets_init: creating a residency set collection (keep_alive = 180 s)
ggml_metal_device_init: GPU name:   MTL0
ggml_metal_device_init: GPU family: MTLGPUFamilyApple9  (1009)
ggml_metal_device_init: GPU family: MTLGPUFamilyCommon3 (3003)
ggml_metal_device_init: GPU family: MTLGPUFamilyMetal4  (5002)
ggml_metal_device_init: simdgroup reduction   = true
ggml_metal_device_init: simdgroup matrix mul. = true
ggml_metal_device_init: has unified memory    = true
ggml_metal_device_init: has bfloat            = true
ggml_metal_device_init: has tensor            = false
ggml_metal_device_init: use residency sets    = true
ggml_metal_device_init: use shared buffers    = true
ggml_metal_device_init: recommendedMaxWorkingSetSize  = 40200.90 MB
Available devices:
  MTL0: Apple M4 Pro (38338 MiB, 38338 MiB free)
  BLAS: Accelerate (0 MiB, 0 MiB free)`,
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "MTL0",
				Name:      "Apple M4 Pro",
				Backend:   BackendMetal,
				VRAMBytes: 38338 * mibToBytes,
			},
		},
		{
			name: "dual CUDA GPUs",
			output: `Available devices:
  CUDA0: NVIDIA GeForce RTX 4090 (24564 MiB, 24000 MiB free)
  CUDA1: NVIDIA GeForce RTX 4090 (24564 MiB, 23500 MiB free)`,
			wantCount: 2,
			wantFirst: GPUDevice{
				ID:        "CUDA0",
				Name:      "NVIDIA GeForce RTX 4090",
				Backend:   BackendCUDA,
				VRAMBytes: 24564 * mibToBytes,
			},
		},
		{
			name: "vulkan device",
			output: `Available devices:
  VK0: AMD Radeon RX 7900 XTX (24560 MiB, 24000 MiB free)`,
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "VK0",
				Name:      "AMD Radeon RX 7900 XTX",
				Backend:   BackendVulkan,
				VRAMBytes: 24560 * mibToBytes,
			},
		},
		{
			name: "ROCm device",
			output: `Available devices:
  ROCm0: AMD Radeon PRO W7900 (49140 MiB, 48500 MiB free)`,
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "ROCm0",
				Name:      "AMD Radeon PRO W7900",
				Backend:   BackendROCm,
				VRAMBytes: 49140 * mibToBytes,
			},
		},
		{
			name: "dual ROCm GPUs",
			output: `Available devices:
  ROCm0: AMD Instinct MI300X (196608 MiB, 190000 MiB free)
  ROCm1: AMD Instinct MI300X (196608 MiB, 189000 MiB free)`,
			wantCount: 2,
			wantFirst: GPUDevice{
				ID:        "ROCm0",
				Name:      "AMD Instinct MI300X",
				Backend:   BackendROCm,
				VRAMBytes: 196608 * mibToBytes,
			},
		},
		{
			name:      "no marker best effort",
			output:    `  CUDA0: NVIDIA A100 (81920 MiB, 81000 MiB free)`,
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "CUDA0",
				Name:      "NVIDIA A100",
				Backend:   BackendCUDA,
				VRAMBytes: 81920 * mibToBytes,
			},
		},
		{
			name: "float MiB values",
			output: `Available devices:
  MTL0: Apple M2 Ultra (196608.5 MiB, 190000.0 MiB free)`,
			wantCount: 1,
			wantFirst: GPUDevice{
				ID:        "MTL0",
				Name:      "Apple M2 Ultra",
				Backend:   BackendMetal,
				VRAMBytes: 196609 * mibToBytes,
			},
		},
		{
			name:    "empty output",
			output:  "",
			wantErr: true,
		},
		{
			name: "all zero MiB",
			output: `Available devices:
  BLAS: Accelerate (0 MiB, 0 MiB free)`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devices, err := parseLlamaBenchDevices(tt.output)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(devices) != tt.wantCount {
				t.Fatalf("expected %d devices, got %d", tt.wantCount, len(devices))
			}

			got := devices[0]
			if got.ID != tt.wantFirst.ID {
				t.Errorf("ID: got %q, want %q", got.ID, tt.wantFirst.ID)
			}
			if got.Name != tt.wantFirst.Name {
				t.Errorf("Name: got %q, want %q", got.Name, tt.wantFirst.Name)
			}
			if got.Backend != tt.wantFirst.Backend {
				t.Errorf("Backend: got %q, want %q", got.Backend, tt.wantFirst.Backend)
			}
			if got.VRAMBytes != tt.wantFirst.VRAMBytes {
				t.Errorf("VRAMBytes: got %d, want %d", got.VRAMBytes, tt.wantFirst.VRAMBytes)
			}
		})
	}
}

func TestInferBackend(t *testing.T) {
	tests := []struct {
		id   string
		want GPUBackend
	}{
		{"MTL0", BackendMetal},
		{"MTL1", BackendMetal},
		{"CUDA0", BackendCUDA},
		{"CUDA1", BackendCUDA},
		{"ROCm0", BackendROCm},
		{"ROCm1", BackendROCm},
		{"VK0", BackendVulkan},
		{"VK1", BackendVulkan},
		{"GPU0", BackendUnknown},
		{"0", BackendUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := inferBackend(tt.id)
			if got != tt.want {
				t.Errorf("inferBackend(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}

func TestHardwareInfoHelpers(t *testing.T) {
	info := HardwareInfo{
		GPUs: []GPUDevice{
			{ID: "CUDA0", VRAMBytes: 24564 * mibToBytes},
			{ID: "CUDA1", VRAMBytes: 24564 * mibToBytes},
		},
	}

	if info.GPUCount() != 2 {
		t.Errorf("GPUCount: got %d, want 2", info.GPUCount())
	}

	wantTotal := uint64(24564*mibToBytes) * 2
	if info.TotalVRAMBytes() != wantTotal {
		t.Errorf("TotalVRAMBytes: got %d, want %d", info.TotalVRAMBytes(), wantTotal)
	}
}

func TestHardwareInfoEmpty(t *testing.T) {
	var info HardwareInfo

	if info.GPUCount() != 0 {
		t.Errorf("GPUCount: got %d, want 0", info.GPUCount())
	}

	if info.TotalVRAMBytes() != 0 {
		t.Errorf("TotalVRAMBytes: got %d, want 0", info.TotalVRAMBytes())
	}
}
