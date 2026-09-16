package model

// This file contains workarounds for Yzma behavior that cannot be implemented
// upstream. Keep InitYzmaWorkarounds as the initialization hook for future
// Kronk-specific compatibility code.
//
// Current speculative bindings are owned by
// github.com/hybridgroup/yzma/exp/speculative, so there are no local
// workarounds to initialize.

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
// It is intentionally a no-op until Kronk requires another local workaround.
func InitYzmaWorkarounds(libPath string) error {
	_ = libPath

	return nil
}
