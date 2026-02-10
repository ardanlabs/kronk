//go:build linux

package hardware

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func totalRAMBytes() (uint64, error) {
	total, err := memTotalFromProc()
	if err != nil {
		return 0, err
	}

	if capped := applyCgroupMemoryLimit(total); capped < total {
		return capped, nil
	}

	return total, nil
}

func memTotalFromProc() (uint64, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("open /proc/meminfo: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}

		var kB uint64
		if _, err := fmt.Sscanf(line, "MemTotal: %d", &kB); err != nil {
			return 0, fmt.Errorf("parse MemTotal: %w", err)
		}

		return kB * 1024, nil
	}

	return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
}

func applyCgroupMemoryLimit(hostTotal uint64) uint64 {
	// cgroup v2
	if val, err := readUint64File("/sys/fs/cgroup/memory.max"); err == nil && val < hostTotal {
		return val
	}

	// cgroup v1
	if val, err := readUint64File("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil && val < hostTotal {
		return val
	}

	return hostTotal
}

func readUint64File(path string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	s := strings.TrimSpace(string(data))
	if s == "max" || s == "" {
		return 0, fmt.Errorf("no numeric limit")
	}

	return strconv.ParseUint(s, 10, 64)
}
