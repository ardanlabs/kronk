//go:build darwin

package diagnose

import (
	"strconv"
	"strings"
)

// parseSystem extracts the CPU model and total RAM (bytes) from the captured
// macOS commands. The values come from the "sysctl -n machdep.cpu.brand_string
// hw.memsize ..." output, whose first two lines are the brand string and the
// memory size in bytes.
func parseSystem(cmds []Command) (cpuModel string, ramBytes uint64) {
	lines := strings.Split(strings.TrimSpace(commandOutput(cmds, "machdep.cpu.brand_string")), "\n")
	if len(lines) > 0 {
		cpuModel = strings.TrimSpace(lines[0])
	}
	if len(lines) > 1 {
		ramBytes, _ = strconv.ParseUint(strings.TrimSpace(lines[1]), 10, 64)
	}
	return cpuModel, ramBytes
}

// gpuAccessHints returns no hints on macOS; Metal device access is not gated
// by render-node permissions.
func gpuAccessHints() []Hint {
	return nil
}

// systemCommandSpecs returns the host/device commands to capture on macOS.
func systemCommandSpecs() []commandSpec {
	return []commandSpec{
		// OS version + kernel.
		{"sw_vers", nil},
		{"uname", []string{"-a"}},

		// CPU / memory / hardware from the kernel.
		{"sysctl", []string{"-n", "machdep.cpu.brand_string", "hw.memsize", "hw.logicalcpu", "hw.physicalcpu"}},

		// Memory pressure / usage right now. Swap usage and free disk are the
		// two most common real problems: a model bigger than RAM swaps (and
		// feels like a Kronk bug), and a full disk makes downloads fail.
		{"vm_stat", nil},
		{"sysctl", []string{"vm.swapusage"}},
		{"df", []string{"-h"}},

		// Power / thermal state (no sudo needed). Reveals whether the machine
		// is throttled, on battery, or in Low Power Mode.
		{"pmset", []string{"-g", "therm"}}, // CPU_Speed_Limit < 100 => throttling
		{"pmset", []string{"-g", "batt"}},  // AC vs battery, charge %
		{"pmset", []string{"-g"}},          // power settings incl. lowpowermode
	}
}
