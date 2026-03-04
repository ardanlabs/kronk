//go:build !darwin && !linux

package toolapp

func systemRAMBytes() uint64 {
	return 0
}
