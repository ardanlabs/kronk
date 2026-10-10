package pool

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	"github.com/ardanlabs/kronk/sdk/tools/devices"
)

// preparePlacement freezes device order and implicit split weights so planning
// and native loading use the same placement inputs. User configs are not mutated.
func (l *Llama) preparePlacement(cfg model.Config) (model.Config, error) {
	if !l.resman.HasGPUs() || l.resman.UnifiedMemory() {
		return cfg, nil
	}
	available := l.startupDevices.Devices
	if devices.Ready() {
		available = devices.List().Devices
	}
	if len(available) == 0 {
		for _, device := range l.resman.Usage().Devices {
			available = append(available, devices.DeviceInfo{Name: device.Name, Type: device.Type, FreeBytes: uint64(max(device.BudgetBytes-device.UsedBytes, 0)), TotalBytes: uint64(device.TotalBytes)})
		}
	}
	gpus := slices.DeleteFunc(slices.Clone(available), func(device devices.DeviceInfo) bool {
		return !strings.HasPrefix(device.Type, "gpu_")
	})

	var err error
	if cfg.NGpuLayers() != -1 {
		cfg.Devices, cfg.TensorSplit, err = resolvePlacement(gpus, cfg.Devices, cfg.TensorSplit)
		if err != nil {
			return cfg, err
		}
		main := cfg.MainGPU()
		if cfg.PtrSplitMode != nil && *cfg.PtrSplitMode == model.SplitModeNone {
			if main < 0 {
				cfg.PtrNGpuLayers = new(-1)
			} else if main >= len(cfg.Devices) {
				return cfg, fmt.Errorf("placement: main GPU[%d] outside selected devices: %w", main, resman.ErrInvalidPlan)
			} else {
				cfg.Devices = []string{cfg.Devices[main]}
				cfg.TensorSplit = nil
				cfg.PtrMainGPU = new(0)
			}
		} else if main < 0 || main >= len(cfg.Devices) {
			return cfg, fmt.Errorf("placement: main GPU[%d] outside selected devices: %w", main, resman.ErrInvalidPlan)
		}
	}
	if cfg.PtrDraftModel != nil && cfg.PtrDraftModel.IsSeparate() && cfg.SpeculationMode() != model.SpeculationDisabled && cfg.PtrDraftModel.NGpuLayers() != -1 {
		draft := *cfg.PtrDraftModel
		draft.Devices, draft.TensorSplit, err = resolvePlacement(gpus, draft.Devices, draft.TensorSplit)
		if err != nil {
			return cfg, fmt.Errorf("draft: %w", err)
		}
		if draft.MainGPU() < 0 || draft.MainGPU() >= len(draft.Devices) {
			return cfg, fmt.Errorf("placement: draft main GPU outside selected devices: %w", resman.ErrInvalidPlan)
		}
		cfg.PtrDraftModel = &draft
	}
	if cfg.ProjFile != "" && (cfg.PtrProjOnCPU == nil || !*cfg.PtrProjOnCPU) && cfg.ProjDevice == "" && len(gpus) > 0 {
		// mtmd's default projector uses the first registered GPU, not the
		// target model's selected device or main_gpu, with an IGPU fallback.
		index := max(slices.IndexFunc(gpus, func(device devices.DeviceInfo) bool { return device.HardwareType != "IGPU" }), 0)
		cfg.ProjDevice = gpus[index].Name
	}
	return cfg, nil
}

func resolvePlacement(available []devices.DeviceInfo, names []string, split []float32) ([]string, []float32, error) {
	names = slices.Clone(names)
	if len(names) == 0 {
		// Native automatic placement prefers RPC devices, then local discrete
		// GPUs. Integrated GPUs are used only without local discrete GPUs.
		hasDiscrete := slices.ContainsFunc(available, func(device devices.DeviceInfo) bool {
			return device.HardwareType != "IGPU" && device.Backend != "RPC"
		})
		for _, device := range available {
			if device.Backend == "RPC" {
				names = append(names, device.Name)
			}
		}
		for _, device := range available {
			if device.Backend != "RPC" && (!hasDiscrete || device.HardwareType != "IGPU") {
				names = append(names, device.Name)
			}
		}
	}
	if len(split) > 0 && len(split) != len(names) {
		return nil, nil, fmt.Errorf("placement: devices[%d] != tensor split[%d]: %w", len(names), len(split), resman.ErrInvalidPlan)
	}
	var sum float32
	for _, weight := range split {
		if weight < 0 || math.IsNaN(float64(weight)) || math.IsInf(float64(weight), 0) {
			return nil, nil, fmt.Errorf("placement: invalid tensor split: %w", resman.ErrInvalidPlan)
		}
		sum += weight
	}
	weights := slices.Clone(split)
	if sum == 0 {
		weights = make([]float32, len(names))
	}
	for i, name := range names {
		index := slices.IndexFunc(available, func(device devices.DeviceInfo) bool { return device.Name == name })
		if index < 0 {
			return nil, nil, fmt.Errorf("placement: device[%s]: %w", name, resman.ErrUnknownDevice)
		}
		if slices.Contains(names[:i], name) {
			return nil, nil, fmt.Errorf("placement: duplicate device[%s]: %w", name, resman.ErrInvalidPlan)
		}
		if sum == 0 {
			weights[i] = float32(available[index].FreeBytes)
		}
	}
	if sum == 0 {
		var free float32
		for _, weight := range weights {
			free += weight
		}
		if free == 0 {
			// A full resident cache may be evicted after planning. Freeze a
			// capacity-weighted split rather than rejecting before eviction.
			for i, name := range names {
				index := slices.IndexFunc(available, func(device devices.DeviceInfo) bool { return device.Name == name })
				weights[i] = float32(available[index].TotalBytes)
				free += weights[i]
			}
			if free == 0 {
				return nil, nil, fmt.Errorf("placement: no GPU capacity available: %w", resman.ErrNoGPUs)
			}
		}
	}
	return names, weights, nil
}

// addResult maps the calculator's components to physical resource pools. Only
// the calculator derives per-device byte counts; resman receives named amounts.
func (l *Llama) addResult(plan *resman.PlanRequest, result vram.Result, gpu bool, names []string, split []float32, main int) {
	switch {
	case l.resman.UnifiedMemory():
		plan.RAMBytes += result.UnifiedFootprint()
	case !l.resman.HasGPUs() || !gpu:
		plan.RAMBytes += result.TotalVRAM + result.TotalSystemRAMEst
	default:
		plan.RAMBytes += result.TotalSystemRAMEst
		ratios := make([]float64, len(split))
		for i, weight := range split {
			ratios[i] = float64(weight)
		}
		for i, device := range result.CalculatePerDevice(int64(len(names)), ratios, names, main) {
			addDeviceBytes(plan, names[i], device.TotalBytes)
		}
	}
}

func addDeviceBytes(plan *resman.PlanRequest, name string, bytes int64) {
	index := slices.Index(plan.Devices, name)
	if index < 0 {
		plan.Devices = append(plan.Devices, name)
		plan.DeviceBytes = append(plan.DeviceBytes, bytes)
	} else {
		plan.DeviceBytes[index] += bytes
	}
	plan.VRAMBytes += bytes
}
