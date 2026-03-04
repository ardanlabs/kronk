//go:build linux

package toolapp

import "golang.org/x/sys/unix"

func systemRAMBytes() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return info.Totalram * uint64(info.Unit)
}
