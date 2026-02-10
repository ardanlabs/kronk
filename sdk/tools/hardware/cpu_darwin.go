//go:build darwin

package hardware

import "syscall"

func cpuPackages() int {
	val, err := syscall.SysctlUint32("hw.packages")
	if err != nil || val == 0 {
		return 1
	}

	return int(val)
}

func logicalCores() int {
	val, err := syscall.SysctlUint32("hw.logicalcpu")
	if err != nil || val == 0 {
		return 1
	}

	return int(val)
}
