//go:build linux

package diagnose

import (
	"strconv"
	"strings"
)

// parseSystem extracts the CPU model and total RAM (bytes) from the captured
// Linux commands. CPU comes from the "Model name:" line of lscpu; RAM comes
// from the "MemTotal:" line of /proc/meminfo, which reports kibibytes.
func parseSystem(cmds []Command) (cpuModel string, ramBytes uint64) {
	for line := range strings.SplitSeq(commandOutput(cmds, "lscpu"), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "Model name:"); ok {
			cpuModel = strings.TrimSpace(rest)
			break
		}
	}

	for line := range strings.SplitSeq(commandOutput(cmds, "meminfo"), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "MemTotal:"); ok {
			fields := strings.Fields(rest) // e.g. "16384000 kB"
			if len(fields) > 0 {
				kb, _ := strconv.ParseUint(fields[0], 10, 64)
				ramBytes = kb * 1024
			}
			break
		}
	}

	return cpuModel, ramBytes
}

// systemCommandSpecs returns the host/device commands to capture on Linux,
// including discrete GPUs. GPU tools only exist when the matching drivers are
// installed; capture() records an error otherwise.
func systemCommandSpecs() []commandSpec {
	return []commandSpec{
		// OS version + kernel.
		{"uname", []string{"-a"}},
		{"cat", []string{"/etc/os-release"}},

		// CPU / memory.
		{"lscpu", nil},
		{"cat", []string{"/proc/meminfo"}},
		{"free", []string{"-h"}},

		// Swap + free disk (same "model bigger than RAM / disk full" failures).
		{"swapon", []string{"--show"}},
		{"df", []string{"-h"}},

		// GPU cards. This is the important one for the "Linux with GPU cards"
		// target.
		{"nvidia-smi", nil}, // NVIDIA
		{"rocm-smi", nil},   // AMD
	}
}
