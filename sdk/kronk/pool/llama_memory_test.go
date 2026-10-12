package pool

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/loader"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestPlanFallbackReservesUnifiedModelWeights(t *testing.T) {
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mdls.Path(), "test", "family")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// A readable GGUF without sizing metadata must take the file-size fallback.
	var data bytes.Buffer
	for _, value := range []any{gguf.Magic, uint32(3), uint64(0), uint64(0)} {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "fallback.gguf"), data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	log := func(context.Context, string, ...any) {}
	if err := mdls.BuildIndex(log, false); err != nil {
		t.Fatal(err)
	}
	rm, err := resman.New(resman.Config{Snapshot: resman.Snapshot{RAMBytes: 1 << 30, UnifiedMemory: true}})
	if err != nil {
		t.Fatal(err)
	}
	l := newLlama(log, mdls, nil, rm, devices.Devices{}, false)
	plan, err := l.Plan(context.Background(), loader.LoadRequest{Key: "fallback", ModelID: "test/fallback", Prepared: model.Config{PtrSWAFull: new(true)}})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RAMBytes != int64(data.Len()) || plan.VRAMBytes != 0 {
		t.Fatalf("fallback reservation: got RAM/VRAM %d/%d, want %d/0", plan.RAMBytes, plan.VRAMBytes, data.Len())
	}
}

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
	if checks[0].enforced || !checks[0].exhausted {
		t.Errorf("Metal check: got enforced=%t exhausted=%t, want false true", checks[0].enforced, checks[0].exhausted)
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
		{value: 115448725504, want: "107.5GiB"},
		{value: 11813308006, want: "11.0GiB"},
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
		{name: "Metal advisory", deviceType: "gpu_metal", wantCheck: true, wantEnforced: false},
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

	additional := func(l *Llama, cfg model.Config) (int64, int64, error) {
		cfg, err := l.preparePlacement(cfg)
		if err != nil {
			return 0, 0, err
		}
		var plan resman.PlanRequest
		err = l.additionalMemory(&plan, cfg, vram.Config{}, vram.Result{})
		return plan.VRAMBytes, plan.RAMBytes, err
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
	vramBytes, ramBytes, err := additional(discrete, model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	})
	if err != nil {
		t.Fatalf("additionalMemory discrete: %v", err)
	}
	if vramBytes != 300 || ramBytes != 0 {
		t.Errorf("discrete bytes: got VRAM=%d RAM=%d, want VRAM=300 RAM=0", vramBytes, ramBytes)
	}

	vramBytes, ramBytes, err = additional(discrete, model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
		Speculation:    model.SpeculationDisabled,
	})
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
	vramBytes, ramBytes, err = additional(unified, model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	})
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
	vramBytes, ramBytes, err = additional(cpu, model.Config{
		ProjFile:       projection,
		MTPDrafterFile: mtp,
	})
	if err != nil {
		t.Fatalf("additionalMemory CPU: %v", err)
	}
	if vramBytes != 0 || ramBytes != 300 {
		t.Errorf("CPU bytes: got VRAM=%d RAM=%d, want VRAM=0 RAM=300", vramBytes, ramBytes)
	}
}

func TestPlanLlamaProjectorWorkspace(t *testing.T) {
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mdls.Path(), "test", "family")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var projectorSize int64
	for name, metadata := range map[string]map[string]string{
		"target":    {"general.architecture": "llama", "llama.block_count": "2", "llama.embedding_length": "64", "llama.attention.head_count": "1", "llama.attention.head_count_kv": "1"},
		"projector": {"general.architecture": "clip", "clip.projector_type": "ultravox"},
	} {
		var header bytes.Buffer
		write := func(value any) {
			t.Helper()
			if err := binary.Write(&header, binary.LittleEndian, value); err != nil {
				t.Fatal(err)
			}
		}
		writeString := func(value string) {
			write(uint64(len(value)))
			if _, err := header.WriteString(value); err != nil {
				t.Fatal(err)
			}
		}
		write(gguf.Magic)
		write(uint32(3))
		write(uint64(0))
		write(uint64(len(metadata)))
		for key, value := range metadata {
			writeString(key)
			write(gguf.MetadataValueTypeString)
			writeString(value)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".gguf"), header.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		if name == "projector" {
			projectorSize = int64(header.Len())
		}
	}
	log := func(context.Context, string, ...any) {}
	if err := mdls.BuildIndex(log, false); err != nil {
		t.Fatal(err)
	}
	rm, err := resman.New(resman.Config{Snapshot: resman.Snapshot{RAMBytes: 64 << 30, UnifiedMemory: true}})
	if err != nil {
		t.Fatal(err)
	}
	l := newLlama(log, mdls, nil, rm, devices.Devices{}, false)
	cfg := model.Config{PtrContextWindow: new(256), PtrNSeqMax: new(1), PtrSWAFull: new(true)}
	base, err := l.Plan(context.Background(), loader.LoadRequest{Key: "model", ModelID: "test/target", Prepared: cfg})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ProjFile = filepath.Join(dir, "projector.gguf")
	withProjector, err := l.Plan(context.Background(), loader.LoadRequest{Key: "model", ModelID: "test/target", Prepared: cfg})
	if err != nil {
		t.Fatal(err)
	}
	// Projector residency plus the existing 256 MiB runtime floor and
	// its 10% margin, independent of the decoder's workspace policy.
	const runtime int64 = 256 << 20
	want := projectorSize + runtime + runtime/10
	if got := withProjector.RAMBytes - base.RAMBytes; got != want {
		t.Fatalf("projector unified reservation: got %d, want %d", got, want)
	}
}

func TestPlanQwen35CompanionReservations(t *testing.T) {
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mdls.Path(), "test", "family")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, name := range []string{"target", "mtp", "projector"} {
		metadata := map[string]string{"general.architecture": "clip"}
		if name != "projector" {
			metadata = map[string]string{
				"general.architecture": "qwen35moe", "qwen35moe.block_count": "41",
				"qwen35moe.embedding_length": "2048", "qwen35moe.attention.head_count": "16",
				"qwen35moe.attention.head_count_kv": "2", "qwen35moe.attention.key_length": "256",
				"qwen35moe.attention.value_length": "256", "qwen35moe.nextn_predict_layers": "1",
			}
		}
		var header bytes.Buffer
		write := func(value any) {
			t.Helper()
			if err := binary.Write(&header, binary.LittleEndian, value); err != nil {
				t.Fatal(err)
			}
		}
		writeString := func(value string) {
			write(uint64(len(value)))
			if _, err := header.WriteString(value); err != nil {
				t.Fatal(err)
			}
		}
		write(gguf.Magic)
		write(uint32(3))
		write(uint64(0))
		write(uint64(len(metadata)))
		for key, value := range metadata {
			writeString(key)
			write(gguf.MetadataValueTypeString)
			writeString(value)
		}
		path := filepath.Join(dir, name+".gguf")
		if err := os.WriteFile(path, header.Bytes(), 0600); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	log := func(context.Context, string, ...any) {}
	if err := mdls.BuildIndex(log, false); err != nil {
		t.Fatal(err)
	}
	rm, err := resman.New(resman.Config{Snapshot: resman.Snapshot{RAMBytes: 64 << 30, UnifiedMemory: true}})
	if err != nil {
		t.Fatal(err)
	}
	l := newLlama(log, mdls, nil, rm, devices.Devices{}, false)
	cfg := model.Config{ModelFiles: files[:1], MTPDrafterFile: files[1], ProjFile: files[2], PtrContextWindow: new(262144), PtrNSeqMax: new(1), PtrSWAFull: new(true)}
	plan, err := l.Plan(context.Background(), loader.LoadRequest{Key: "model", ModelID: "test/target", Prepared: cfg})
	if err != nil {
		t.Fatal(err)
	}
	calcCfg := vram.Config{ComputeBackend: devices.Backend(), ContextWindow: 262144, Slots: 1, BytesPerElement: 2, NUBatch: effectivePrefillBatchSize(cfg), SWAFull: true, RecurrentStateCopies: model.RecurrentStateCopies(cfg, false)}
	var want int64
	for i, path := range files {
		calcCfg.MTPContext = i == 1
		result, err := vram.FromFiles([]string{path}, calcCfg)
		if err != nil {
			t.Fatal(err)
		}
		if i == 1 && result.SlotMemory != 512<<20 {
			t.Fatalf("companion cache: got %d, want %d", result.SlotMemory, 512<<20)
		}
		want += result.UnifiedFootprint()
	}
	if plan.RAMBytes != want || plan.VRAMBytes != 0 {
		t.Fatalf("unified reservation: got RAM/VRAM %d/%d, want %d/0", plan.RAMBytes, plan.VRAMBytes, want)
	}
}

func TestPlanReservesCalculatorDeviceResults(t *testing.T) {
	mdls, err := models.NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mdls.Path(), "test", "family")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var header bytes.Buffer
	write := func(value any) {
		t.Helper()
		if err := binary.Write(&header, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	writeString := func(value string) {
		write(uint64(len(value)))
		if _, err := header.WriteString(value); err != nil {
			t.Fatal(err)
		}
	}
	write(gguf.Magic)
	write(uint32(3))
	write(uint64(0))
	write(uint64(5))
	writeString("general.architecture")
	write(gguf.MetadataValueTypeString)
	writeString("qwen3")
	for key, value := range map[string]uint32{"block_count": 4, "attention.head_count": 1, "attention.key_length": 8, "attention.value_length": 4} {
		writeString("qwen3." + key)
		write(gguf.MetadataValueTypeUInt32)
		write(value)
	}
	path := filepath.Join(dir, "placement.gguf")
	if err := os.WriteFile(path, header.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	log := func(context.Context, string, ...any) {}
	if err := mdls.BuildIndex(log, false); err != nil {
		t.Fatal(err)
	}
	base := model.Config{ModelFiles: []string{path}, PtrContextWindow: new(4097), PtrNSeqMax: new(1), PtrSWAFull: new(true)}
	calcCfg := vram.Config{ContextWindow: 4097, BytesPerElement: 2, Slots: 1, SWAFull: true}
	result, err := vram.FromFiles([]string{path}, calcCfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		cfg      model.Config
		unified  bool
		capacity int64
		want     map[string]int64
		wantRAM  int64
		wantErr  error
	}{
		{"explicit reordered/main GPU", model.Config{Devices: []string{"CUDA1", "CUDA0"}, TensorSplit: []float32{1, 3}, PtrMainGPU: new(1)}, false, 1 << 30,
			map[string]int64{"CUDA1": result.ModelWeightsGPU/4 + result.KVVRAMBytes/4, "CUDA0": result.TotalVRAM - result.ModelWeightsGPU/4 - result.KVVRAMBytes/4}, result.TotalSystemRAMEst, nil},
		{"implicit free-memory split", model.Config{}, false, 1 << 30,
			map[string]int64{"CUDA0": result.ModelWeightsGPU*3/4 + result.KVVRAMBytes*3/4 + result.ComputeBufferEst, "CUDA1": result.TotalVRAM - result.ModelWeightsGPU*3/4 - result.KVVRAMBytes*3/4 - result.ComputeBufferEst}, result.TotalSystemRAMEst, nil},
		{"single GPU honors main within reordered list", model.Config{Devices: []string{"CUDA1", "CUDA0"}, PtrMainGPU: new(1), PtrSplitMode: new(model.SplitModeNone)}, false, 1 << 30,
			map[string]int64{"CUDA0": result.TotalVRAM}, result.TotalSystemRAMEst, nil},
		{"independent draft and projector devices", model.Config{Devices: []string{"CUDA0"}, ProjFile: path, ProjDevice: "CUDA1", PtrDraftModel: &model.DraftModelConfig{ModelFiles: []string{path}, Devices: []string{"CUDA1"}}}, false, 1 << 30,
			map[string]int64{"CUDA0": result.TotalVRAM, "CUDA1": result.TotalVRAM + int64(header.Len())}, 2 * result.TotalSystemRAMEst, nil},
		{"default projector independent of target", model.Config{Devices: []string{"CUDA1"}, ProjFile: path}, false, 1 << 30,
			map[string]int64{"CUDA1": result.TotalVRAM, "CUDA0": int64(header.Len())}, result.TotalSystemRAMEst, nil},
		{"CPU model on discrete system", model.Config{PtrNGpuLayers: new(-1)}, false, 1 << 30,
			map[string]int64{}, result.TotalVRAM + result.TotalSystemRAMEst, nil},
		{"compute alone exceeds main GPU", model.Config{Devices: []string{"CUDA1", "CUDA0"}, TensorSplit: []float32{1, 3}, PtrMainGPU: new(1)}, false, result.ComputeBufferEst - 1,
			map[string]int64{"CUDA1": result.ModelWeightsGPU/4 + result.KVVRAMBytes/4, "CUDA0": result.TotalVRAM - result.ModelWeightsGPU/4 - result.KVVRAMBytes/4}, result.TotalSystemRAMEst, resman.ErrNoCapacity},
		{"unified footprint", model.Config{}, true, 1 << 30, map[string]int64{}, result.UnifiedFootprint(), nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rm, err := resman.New(resman.Config{Snapshot: resman.Snapshot{Devices: []resman.Device{{Name: "IGPU", Type: "gpu_vulkan", TotalBytes: 1 << 30}, {Name: "CUDA0", Type: "gpu_cuda", TotalBytes: tt.capacity}, {Name: "CUDA1", Type: "gpu_cuda", TotalBytes: 2 << 30}}, RAMBytes: 8 << 30, UnifiedMemory: tt.unified}, BudgetPercent: 100, HeadroomBytes: -1})
			if err != nil {
				t.Fatal(err)
			}
			l := newLlama(log, mdls, nil, rm, devices.Devices{Devices: []devices.DeviceInfo{{Name: "IGPU", Type: "gpu_vulkan", HardwareType: "IGPU", FreeBytes: 1 << 30, TotalBytes: 1 << 30}, {Name: "CUDA0", Type: "gpu_cuda", FreeBytes: 9 << 28, TotalBytes: uint64(tt.capacity)}, {Name: "CUDA1", Type: "gpu_cuda", FreeBytes: 3 << 28, TotalBytes: 2 << 30}}}, false)
			cfg := tt.cfg
			cfg.ModelFiles, cfg.PtrContextWindow, cfg.PtrNSeqMax, cfg.PtrSWAFull = base.ModelFiles, base.PtrContextWindow, base.PtrNSeqMax, base.PtrSWAFull
			prepared, err := l.Prepare(context.Background(), loader.LoadRequest{Custom: cfg})
			if err != nil {
				t.Fatal(err)
			}
			planReq, err := l.Plan(context.Background(), loader.LoadRequest{Key: "model", ModelID: "test/placement", Prepared: prepared})
			if err != nil {
				t.Fatal(err)
			}
			got := make(map[string]int64)
			for i, name := range planReq.Devices {
				got[name] = planReq.DeviceBytes[i]
			}
			if !reflect.DeepEqual(got, tt.want) || planReq.RAMBytes != tt.wantRAM {
				t.Fatalf("reservation: got %v RAM=%d, want %v RAM=%d", got, planReq.RAMBytes, tt.want, tt.wantRAM)
			}
			if err := rm.VRAMFeasible(planReq); !errors.Is(err, tt.wantErr) {
				t.Fatalf("feasible: got %v, want %v", err, tt.wantErr)
			}
			ticket, plan, err := rm.Reserve(planReq)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("reserve: got %v, want %v", err, tt.wantErr)
			}
			if err == nil {
				for _, allocation := range plan.Per {
					if allocation.Bytes != tt.want[allocation.Name] {
						t.Errorf("reserved[%s]: got %d, want %d", allocation.Name, allocation.Bytes, tt.want[allocation.Name])
					}
				}
				rm.Release(ticket)
			}
		})
	}
}
