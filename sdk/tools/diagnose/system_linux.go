//go:build linux

package diagnose

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// gpuAccessHints looks for the common case where GPU hardware is present but a
// backend still sees no device because the user lacks permission to open the
// DRM render nodes (e.g. not in the "render" group). It returns a remediation
// hint when that is detected, or nil otherwise. A render node existing implies
// a real render-capable GPU; display-only chips do not create one.
func gpuAccessHints() []Hint {
	nodes, _ := filepath.Glob("/dev/dri/renderD*")
	if len(nodes) == 0 {
		// No render nodes: no GPU to access, so nothing to explain here.
		return nil
	}

	var blocked string
	for _, node := range nodes {
		f, err := os.OpenFile(node, os.O_RDWR, 0)
		if err == nil {
			f.Close()
			continue
		}
		if errors.Is(err, os.ErrPermission) {
			blocked = node
			break
		}
	}

	if blocked == "" {
		return nil
	}

	username := "$USER"
	if u, err := user.Current(); err == nil && u.Username != "" {
		username = u.Username
	}

	return []Hint{{
		Severity: "warn",
		Message: fmt.Sprintf("GPU hardware is present but render node %s is not accessible (permission denied); "+
			"the GPU backend falls back to CPU.", blocked),
		Remedy: fmt.Sprintf("sudo usermod -aG render,video %s   # then log out and back in (or reboot)", username),
	}}
}

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
