package models

import "github.com/ardanlabs/kronk/sdk/tools/hardware"

// FitResult describes whether a model fits on a device type.
type FitResult struct {
	RequiredBytes  uint64
	AvailableBytes uint64
	Fits           bool
}

// FitsOnGPU checks if the model's total VRAM requirement fits within
// the sum of all detected GPU VRAM.
func FitsOnGPU(v VRAM, hw hardware.HardwareInfo) FitResult {
	required := uint64(v.TotalVRAM)
	available := hw.TotalVRAMBytes()

	return FitResult{
		RequiredBytes:  required,
		AvailableBytes: available,
		Fits:           available > 0 && available >= required,
	}
}

// FitsOnCPU checks if the model's total memory requirement fits within
// system RAM.
func FitsOnCPU(v VRAM, hw hardware.HardwareInfo) FitResult {
	required := uint64(v.TotalVRAM)
	available := hw.CPU.TotalRAMBytes

	return FitResult{
		RequiredBytes:  required,
		AvailableBytes: available,
		Fits:           available > 0 && available >= required,
	}
}
