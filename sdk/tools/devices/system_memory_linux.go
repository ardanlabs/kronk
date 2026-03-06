//go:build linux

package devices

import "golang.org/x/sys/unix"

// SystemRAMBytes returns the total system RAM in bytes on Linux.
func SystemRAMBytes() uint64 {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0
	}
	return info.Totalram * uint64(info.Unit)
}
