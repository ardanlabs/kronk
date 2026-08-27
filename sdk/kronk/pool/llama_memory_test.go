package pool

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

func TestBackendMemoryChecks(t *testing.T) {
	const gib = uint64(1 << 30)
	snapshot := devices.Devices{Devices: []devices.DeviceInfo{
		{Name: "Metal", Type: "gpu_metal", Backend: "Metal", FreeBytes: 8 * gib, TotalBytes: 100 * gib},
		{Name: "CUDA0", Type: "gpu_cuda", Backend: "CUDA", FreeBytes: 20 * gib, TotalBytes: 100 * gib},
		{Name: "Vulkan0", Type: "gpu_vulkan", Backend: "Vulkan", FreeBytes: 1 * gib, TotalBytes: 100 * gib},
		{Name: "CPU", Type: "cpu", Backend: "CPU", FreeBytes: 1, TotalBytes: 1},
	}}

	checks := backendMemoryChecks(model.Config{}, snapshot, 90, 0)
	if len(checks) != 3 {
		t.Fatalf("checks: got %d, want 3", len(checks))
	}
	if !checks[0].enforced || !checks[0].exhausted {
		t.Errorf("Metal check: got enforced=%t exhausted=%t, want true true", checks[0].enforced, checks[0].exhausted)
	}
	if !checks[1].enforced || checks[1].exhausted {
		t.Errorf("CUDA check: got enforced=%t exhausted=%t, want true false", checks[1].enforced, checks[1].exhausted)
	}
	if checks[2].enforced || !checks[2].exhausted {
		t.Errorf("Vulkan check: got enforced=%t exhausted=%t, want false true", checks[2].enforced, checks[2].exhausted)
	}
}

func TestBackendMemoryChecksTreatsWrappedAndUnavailableValues(t *testing.T) {
	snapshot := devices.Devices{Devices: []devices.DeviceInfo{
		{Name: "wrapped", Type: "gpu_metal", FreeBytes: 101, TotalBytes: 100},
		{Name: "unavailable", Type: "gpu_cuda"},
	}}

	checks := backendMemoryChecks(model.Config{}, snapshot, 100, -1)
	if len(checks) != 1 {
		t.Fatalf("checks: got %d, want 1", len(checks))
	}
	if !checks[0].exhausted {
		t.Error("wrapped free value: got exhausted=false, want true")
	}
	if !checks[0].wrapped {
		t.Error("wrapped free value: got wrapped=false, want true")
	}
	if got := checks[0].displayFree(); got != 0 {
		t.Errorf("display free: got %d, want 0", got)
	}
}

func TestBackendMemoryBytes(t *testing.T) {
	tests := []struct {
		value uint64
		want  string
	}{
		{value: 0, want: "0B"},
		{value: 115448725504, want: "115.4GB"},
		{value: 11813308006, want: "11.8GB"},
	}

	for _, tt := range tests {
		if got := backendMemoryBytes(tt.value); got != tt.want {
			t.Errorf("backendMemoryBytes(%d): got %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestBackendMemoryCheckPolicies(t *testing.T) {
	tests := []struct {
		name         string
		deviceType   string
		wantCheck    bool
		wantEnforced bool
	}{
		{name: "Metal enforced", deviceType: "gpu_metal", wantCheck: true, wantEnforced: true},
		{name: "CUDA enforced", deviceType: "gpu_cuda", wantCheck: true, wantEnforced: true},
		{name: "ROCm enforced", deviceType: "gpu_rocm", wantCheck: true, wantEnforced: true},
		{name: "Vulkan advisory", deviceType: "gpu_vulkan", wantCheck: true, wantEnforced: false},
		{name: "CPU unsupported", deviceType: "cpu", wantCheck: false},
		{name: "future backend unsupported", deviceType: "gpu_future", wantCheck: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check, ok := backendMemoryCheckForDevice(devices.DeviceInfo{
				Name:       tt.name,
				Type:       tt.deviceType,
				FreeBytes:  50,
				TotalBytes: 100,
			}, 90, 0)
			if ok != tt.wantCheck {
				t.Fatalf("check available: got %t, want %t", ok, tt.wantCheck)
			}
			if ok && check.enforced != tt.wantEnforced {
				t.Errorf("enforced: got %t, want %t", check.enforced, tt.wantEnforced)
			}
		})
	}
}

func TestSelectedBackendDevicesHonorsExplicitAndSingleGPUPlacement(t *testing.T) {
	available := []devices.DeviceInfo{
		{Name: "CUDA0", Type: "gpu_cuda"},
		{Name: "CUDA1", Type: "gpu_cuda"},
	}

	explicit := selectedBackendDevices(model.Config{Devices: []string{"CUDA1"}}, available)
	if len(explicit) != 1 || explicit[0].Name != "CUDA1" {
		t.Fatalf("explicit devices: got %v, want CUDA1", explicit)
	}

	single := selectedBackendDevices(model.Config{
		PtrSplitMode: new(model.SplitModeNone),
		PtrMainGPU:   new(1),
	}, available)
	if len(single) != 1 || single[0].Name != "CUDA1" {
		t.Fatalf("single device: got %v, want CUDA1", single)
	}

	unpinnedTarget := selectedBackendDevices(model.Config{
		ProjDevice: "CUDA1",
	}, available)
	if len(unpinnedTarget) != 2 {
		t.Fatalf("unpinned target with pinned projector: got %v, want both GPUs", unpinnedTarget)
	}

	unpinnedDraft := selectedBackendDevices(model.Config{
		Devices: []string{"CUDA1"},
		PtrDraftModel: &model.DraftModelConfig{
			ModelFiles: []string{"draft.gguf"},
		},
	}, available)
	if len(unpinnedDraft) != 2 {
		t.Fatalf("pinned target with unpinned draft: got %v, want both GPUs", unpinnedDraft)
	}
}

func TestBackendMemoryChecksSkipsCPUOnlyConfiguration(t *testing.T) {
	checks := backendMemoryChecks(model.Config{PtrNGpuLayers: new(-1)}, devices.Devices{
		Devices: []devices.DeviceInfo{{Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 1, TotalBytes: 2}},
	}, 90, 0)
	if len(checks) != 0 {
		t.Errorf("checks: got %d, want 0", len(checks))
	}
}

func TestAdditionalMemoryAccountsForProjectionAndCompanionMTPFiles(t *testing.T) {
	dir := t.TempDir()
	projection := filepath.Join(dir, "projection.gguf")
	mtp := filepath.Join(dir, "mtp.gguf")
	if err := os.WriteFile(projection, make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write projection: %v", err)
	}
	if err := os.WriteFile(mtp, make([]byte, 200), 0o600); err != nil {
		t.Fatalf("write MTP: %v", err)
	}

	discreteRM, err := resman.New(resman.Config{
		Snapshot: resman.Snapshot{
			Devices:  []resman.Device{{Name: "CUDA0", Type: "gpu_cuda", TotalBytes: 1 << 30}},
			RAMBytes: 1 << 30,
		},
	})
	if err != nil {
		t.Fatalf("new discrete resource manager: %v", err)
	}
	discrete := newLlama(func(context.Context, string, ...any) {}, nil, nil, discreteRM, devices.Devices{}, false)
	vramBytes, ramBytes, err := discrete.additionalMemory(model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	}, vram.Config{})
	if err != nil {
		t.Fatalf("additionalMemory discrete: %v", err)
	}
	if vramBytes != 300 || ramBytes != 0 {
		t.Errorf("discrete bytes: got VRAM=%d RAM=%d, want VRAM=300 RAM=0", vramBytes, ramBytes)
	}

	vramBytes, ramBytes, err = discrete.additionalMemory(model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
		Speculation:    model.SpeculationDisabled,
	}, vram.Config{})
	if err != nil {
		t.Fatalf("additionalMemory disabled speculation: %v", err)
	}
	if vramBytes != 100 || ramBytes != 0 {
		t.Errorf("disabled speculation bytes: got VRAM=%d RAM=%d, want VRAM=100 RAM=0", vramBytes, ramBytes)
	}

	unifiedRM, err := resman.New(resman.Config{
		Snapshot: resman.Snapshot{RAMBytes: 1 << 30, UnifiedMemory: true},
	})
	if err != nil {
		t.Fatalf("new unified resource manager: %v", err)
	}
	unified := newLlama(func(context.Context, string, ...any) {}, nil, nil, unifiedRM, devices.Devices{}, false)
	vramBytes, ramBytes, err = unified.additionalMemory(model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	}, vram.Config{})
	if err != nil {
		t.Fatalf("additionalMemory unified: %v", err)
	}
	if vramBytes != 0 || ramBytes != 300 {
		t.Errorf("unified bytes: got VRAM=%d RAM=%d, want VRAM=0 RAM=300", vramBytes, ramBytes)
	}

	cpuRM, err := resman.New(resman.Config{
		Snapshot: resman.Snapshot{RAMBytes: 1 << 30},
	})
	if err != nil {
		t.Fatalf("new CPU resource manager: %v", err)
	}
	cpu := newLlama(func(context.Context, string, ...any) {}, nil, nil, cpuRM, devices.Devices{}, false)
	vramBytes, ramBytes, err = cpu.additionalMemory(model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	}, vram.Config{})
	if err != nil {
		t.Fatalf("additionalMemory CPU: %v", err)
	}
	if vramBytes != 0 || ramBytes != 300 {
		t.Errorf("CPU bytes: got VRAM=%d RAM=%d, want VRAM=0 RAM=300", vramBytes, ramBytes)
	}
}
