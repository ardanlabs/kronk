package gomlx

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gomlx/compute"
)

// ComputeInfo describes the backend and devices GoMLX will use for training.
type ComputeInfo struct {
	Backend     string
	Description string
	Devices     []string
}

// InspectCompute opens the configured GoMLX backend without loading a model.
func InspectCompute() (ComputeInfo, error) {
	if config, configured := os.LookupEnv(compute.ConfigEnvVar); configured {
		name, _, _ := strings.Cut(config, ":")
		if name != "" && !slices.Contains(compute.List(), name) {
			return ComputeInfo{}, fmt.Errorf("GoMLX backend %q is not available", name)
		}
	}

	backend, err := compute.New()
	if err != nil {
		return ComputeInfo{}, fmt.Errorf("open GoMLX backend: %w", err)
	}
	defer backend.Finalize()

	info := ComputeInfo{
		Backend:     backend.Name(),
		Description: backend.Description(),
		Devices:     make([]string, backend.NumDevices()),
	}
	for index := range info.Devices {
		info.Devices[index] = backend.DeviceDescription(compute.DeviceNum(index))
	}
	if len(info.Devices) == 0 {
		return ComputeInfo{}, fmt.Errorf("GoMLX backend %q reported no compute devices", info.Backend)
	}
	return info, nil
}
