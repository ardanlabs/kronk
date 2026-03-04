//go:build darwin

package toolapp

import "golang.org/x/sys/unix"

func systemRAMBytes() uint64 {
	val, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return val
}
