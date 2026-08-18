package pool

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestAutoTuneBudgetUsesStartupAvailability(t *testing.T) {
	rm, err := resman.New(resman.Config{
		Snapshot: resman.Snapshot{
			Devices: []resman.Device{
				{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 1_000},
				{Name: "CUDA1", Type: "gpu_cuda", TotalBytes: 2_000},
			},
			RAMBytes: 1_000,
		},
		BudgetPercent: 100,
		HeadroomBytes: -1,
	})
	if err != nil {
		t.Fatalf("resman.New: %v", err)
	}

	startup := devices.Devices{
		Devices: []devices.DeviceInfo{
			{Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 600, TotalBytes: 1_000},
			{Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 1_000, TotalBytes: 2_000},
		},
		SystemRAMBytes: 800,
	}
	l := Llama{
		resman:         rm,
		startupDevices: startup,
		modelConfig: map[string]models.ModelConfig{
			"selected":     {Devices: []string{"CUDA0"}},
			"single":       {PtrSplitMode: new(model.SplitModeNone)},
			"equal-split":  {TensorSplit: []float32{0.5, 0.5}},
			"device-split": {Devices: []string{"CUDA0", "CUDA1"}, TensorSplit: []float32{0.5, 0.5}},
		},
	}

	tests := []struct {
		name         string
		modelID      string
		wantGPUBytes int64
	}{
		{name: "automatic split", modelID: "unselected", wantGPUBytes: 1_275},
		{name: "selected device", modelID: "selected", wantGPUBytes: 510},
		{name: "split disabled", modelID: "single", wantGPUBytes: 850},
		{name: "automatic devices with equal split", modelID: "equal-split", wantGPUBytes: 1_020},
		{name: "selected devices with equal split", modelID: "device-split", wantGPUBytes: 1_020},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget := l.autoTuneBudget(tt.modelID)
			if budget.GPUBytes != tt.wantGPUBytes {
				t.Errorf("GPUBytes: got %d, want %d", budget.GPUBytes, tt.wantGPUBytes)
			}
			if budget.SystemRAMBytes != 680 {
				t.Errorf("SystemRAMBytes: got %d, want 680", budget.SystemRAMBytes)
			}
		})
	}

	empty := l.autoTuneBudget("unselected")
	if _, _, err := rm.Reserve(resman.PlanRequest{Key: "resident", Devices: []string{"CUDA0"}, VRAMBytes: 100, RAMBytes: 100}); err != nil {
		t.Fatalf("Reserve resident: %v", err)
	}
	resident := l.autoTuneBudget("unselected")
	if resident.GPUBytes != empty.GPUBytes || resident.SystemRAMBytes != empty.SystemRAMBytes {
		t.Errorf("resident budget: got gpu=%d ram=%d, want gpu=%d ram=%d", resident.GPUBytes, resident.SystemRAMBytes, empty.GPUBytes, empty.SystemRAMBytes)
	}
}

func TestCloneDevicesCopiesDeviceSlice(t *testing.T) {
	src := devices.Devices{
		Devices: []devices.DeviceInfo{{Name: "CUDA0"}},
	}
	dst := cloneDevices(src)
	src.Devices[0].Name = "changed"

	if dst.Devices[0].Name != "CUDA0" {
		t.Errorf("device name: got %q, want CUDA0", dst.Devices[0].Name)
	}
}
