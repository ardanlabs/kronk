//go:build !darwin && !linux

package devices

// SystemRAMBytes returns 0 on unsupported platforms.
func SystemRAMBytes() uint64 {
	return 0
}
