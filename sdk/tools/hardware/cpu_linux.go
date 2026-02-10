//go:build linux

package hardware

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func cpuPackages() int {
	if n := cpuPackagesFromSysfs(); n > 0 {
		return n
	}

	if n := cpuPackagesFromProc(); n > 0 {
		return n
	}

	return 1
}

func logicalCores() int {
	cores := logicalCoresFromProc()

	if capped := applyCgroupCPULimit(cores); capped > 0 {
		return capped
	}

	if cores > 0 {
		return cores
	}

	return 1
}

func cpuPackagesFromSysfs() int {
	matches, err := filepath.Glob("/sys/devices/system/cpu/cpu*/topology/physical_package_id")
	if err != nil || len(matches) == 0 {
		return 0
	}

	unique := make(map[string]struct{})
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		unique[strings.TrimSpace(string(data))] = struct{}{}
	}

	return len(unique)
}

func cpuPackagesFromProc() int {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	unique := make(map[string]struct{})
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "physical id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				unique[strings.TrimSpace(parts[1])] = struct{}{}
			}
		}
	}

	return len(unique)
}

func logicalCoresFromProc() int {
	f, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return 0
	}
	defer f.Close()

	var count int
	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.HasPrefix(s.Text(), "processor") {
			count++
		}
	}

	return count
}

func applyCgroupCPULimit(hostCores int) int {
	f, err := os.Open("/sys/fs/cgroup/cpu.max")
	if err != nil {
		return 0
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	if !s.Scan() {
		return 0
	}

	parts := strings.Fields(s.Text())
	if len(parts) != 2 || parts[0] == "max" {
		return 0
	}

	quota, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0
	}

	period, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || period == 0 {
		return 0
	}

	capped := int(quota / period)
	if capped < 1 {
		capped = 1
	}

	if capped < hostCores {
		return capped
	}

	return 0
}

func readFileFirstLine(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func parseUint64(s string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(s), 10, 64)
}

func parseInt(s string) (int, error) {
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0, fmt.Errorf("parse int: %w", err)
	}
	return v, nil
}
