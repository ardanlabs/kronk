//go:build darwin

package hardware

import (
	"encoding/binary"
	"fmt"
	"syscall"
)

func totalRAMBytes() (uint64, error) {
	val, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return 0, fmt.Errorf("sysctl hw.memsize: %w", err)
	}

	if len(val) < 8 {
		return 0, fmt.Errorf("sysctl hw.memsize: unexpected length %d", len(val))
	}

	return binary.LittleEndian.Uint64([]byte(val)), nil
}
