package model

import (
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

var getCausalAttnFunc ffi.Fun

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
func InitYzmaWorkarounds(libPath string) error {
	lib, err := loader.LoadLibrary(libPath, "llama")
	if err != nil {
		return fmt.Errorf("load llama library: %w", err)
	}

	getCausalAttnFunc, err = lib.Prep("llama_get_causal_attn", &ffi.TypeUint8, &ffi.TypePointer)
	if err != nil {
		return fmt.Errorf("prepare llama_get_causal_attn: %w", err)
	}

	return nil
}

// GetCausalAttn reports whether the context is using causal attention.
func GetCausalAttn(ctx llama.Context) bool {
	if ctx == 0 {
		return false
	}

	var causalAttn bool
	getCausalAttnFunc.Call(unsafe.Pointer(&causalAttn), unsafe.Pointer(&ctx))

	return causalAttn
}
