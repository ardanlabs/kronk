package model

import (
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

var batchExtSetDecisionOrderFunc ffi.Fun

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
func InitYzmaWorkarounds(libPath string) error {
	lib, err := loader.LoadLibrary(libPath, "llama")
	if err != nil {
		return fmt.Errorf("load llama library: %w", err)
	}

	// The staging declaration lacks C linkage, so release binaries export
	// platform-specific decorated names.
	for _, symbol := range []string{
		"llama_batch_ext_set_decision_order",
		"_Z34llama_batch_ext_set_decision_orderP15llama_batch_exti20llama_decision_order",
		"?llama_batch_ext_set_decision_order@@YA_NPEAUllama_batch_ext@@HW4llama_decision_order@@@Z",
	} {
		batchExtSetDecisionOrderFunc, err = lib.Prep(symbol, &ffi.TypeUint8,
			&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32)
		if err == nil {
			return nil
		}
	}

	return nil
}

func batchExtDecisionOrderAvailable() bool {
	return batchExtSetDecisionOrderFunc.Addr != 0
}

func batchExtSetDecisionOrder(batch llama.BatchExt, index int32, order int32) bool {
	if batch == 0 || batchExtSetDecisionOrderFunc.Addr == 0 {
		return false
	}

	var result ffi.Arg
	batchExtSetDecisionOrderFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&batch), &index, &order)
	return result.Bool()
}
