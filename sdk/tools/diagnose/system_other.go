//go:build !darwin && !linux && !windows

package diagnose

// systemCommandSpecs returns no commands on unsupported operating systems.
func systemCommandSpecs() []commandSpec {
	return nil
}

// parseSystem reports no parsed values on unsupported operating systems.
func parseSystem(cmds []Command) (cpuModel string, ramBytes uint64) {
	return "", 0
}

// gpuAccessHints returns no hints on unsupported operating systems.
func gpuAccessHints() []Hint {
	return nil
}
