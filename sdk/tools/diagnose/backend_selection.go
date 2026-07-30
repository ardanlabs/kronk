package diagnose

import (
	"fmt"
	"path/filepath"
	"strings"
)

type backendProbeState uint8

const (
	backendProbeUnknown backendProbeState = iota
	backendProbeNone
	backendProbeDevices
)

// backendSelectionHints reports a verified installed accelerator alternative
// when the backend selected by the engine explicitly detects no GPU.
func backendSelectionHints(engine Engine, backends []Backend) []Hint {
	if !engine.Probed {
		return nil
	}

	selected, ok := selectedEngineBackend(engine, backends)
	if !ok || backendDeviceProbeState(selected) != backendProbeNone {
		return nil
	}

	for _, alternative := range backends {
		state, devices := backendDeviceProbe(alternative)
		if alternative.Processor == selected.Processor || alternative.Version != selected.Version || !isAcceleratorBackend(alternative.Processor) || state != backendProbeDevices {
			continue
		}

		var names []string
		for _, device := range devices {
			names = append(names, device.Name)
		}

		return []Hint{{
			Severity: "warn",
			Message: fmt.Sprintf("The selected %s backend detected no GPU, but the installed %s backend detected %s.",
				selected.Processor, alternative.Processor, strings.Join(names, ", ")),
			Remedy: fmt.Sprintf("unset KRONK_LIB_PATH if it is set, set KRONK_PROCESSOR=%s, and restart Kronk", alternative.Processor),
		}}
	}

	return nil
}

func selectedEngineBackend(engine Engine, backends []Backend) (Backend, bool) {
	for _, backend := range backends {
		if engine.LibPath != "" && filepath.Clean(backend.BinDir) == filepath.Clean(engine.LibPath) {
			return backend, true
		}
	}

	for _, backend := range backends {
		if backend.Processor == engine.Processor {
			return backend, true
		}
	}

	return Backend{}, false
}

func backendDeviceProbeState(backend Backend) backendProbeState {
	state, _ := backendDeviceProbe(backend)
	return state
}

func backendDeviceProbe(backend Backend) (backendProbeState, []Device) {
	for _, command := range backend.Commands {
		if !strings.Contains(command.Cmd, "--list-devices") {
			continue
		}
		if command.Err != "" {
			return backendProbeUnknown, nil
		}

		const header = "Available devices:"
		body, ok := backendDeviceBody(command.Output, header)
		if !ok {
			return backendProbeUnknown, nil
		}

		devices := parseDevices(body)
		if len(devices) > 0 && len(devices) == nonEmptyLineCount(body) {
			return backendProbeDevices, devices
		}
		if strings.TrimSpace(body) == "(none)" {
			return backendProbeNone, nil
		}
		return backendProbeUnknown, nil
	}

	return backendProbeUnknown, nil
}

func backendDeviceBody(output string, header string) (string, bool) {
	lines := strings.Split(output, "\n")
	headerLine := -1
	for i, line := range lines {
		if strings.TrimSuffix(line, "\r") == header {
			headerLine = i
		}
	}
	if headerLine < 0 {
		return "", false
	}
	return strings.Join(lines[headerLine+1:], "\n"), true
}

func nonEmptyLineCount(value string) int {
	var count int
	for line := range strings.SplitSeq(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func isAcceleratorBackend(processor string) bool {
	switch processor {
	case "cuda", "metal", "rocm", "vulkan":
		return true
	default:
		return false
	}
}
