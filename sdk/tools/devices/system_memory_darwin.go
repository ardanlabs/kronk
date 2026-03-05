//go:build darwin

package devices

import "golang.org/x/sys/unix"

// SystemRAMBytes returns the total system RAM in bytes on macOS.
func SystemRAMBytes() uint64 {
	val, err := unix.SysctlUint64("hw.memsize")
	if err != nil {
		return 0
	}
	return val
}
