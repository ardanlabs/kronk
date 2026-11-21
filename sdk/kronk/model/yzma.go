// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.

package model

import (
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/jupiterrider/ffi"
)

// Temporary binding until https://github.com/hybridgroup/yzma/issues/508 lands.
var batchExtSetDecisionGroupFunc ffi.Fun

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
func InitYzmaWorkarounds(path string) error {
	lib, err := loader.LoadLibrary(path, "llama")
	if err != nil {
		return fmt.Errorf("load decision group binding: %w", err)
	}
	// llama-ext.h has C++ linkage. Match Yzma's optional decision-order
	// binding: plain C, Itanium (Darwin/Linux), then MSVC (Windows).
	for _, name := range []string{
		"llama_batch_ext_set_decision_group",
		"_Z34llama_batch_ext_set_decision_groupP15llama_batch_extii",
		"?llama_batch_ext_set_decision_group@@YA_NPEAUllama_batch_ext@@HH@Z",
	} {
		if batchExtSetDecisionGroupFunc, err = lib.Prep(name, &ffi.TypeUint8, &ffi.TypePointer, &ffi.TypeSint32, &ffi.TypeSint32); err == nil {
			break
		}
	}
	// Older libraries retain independent-question execution.
	return nil
}

func batchExtSetDecisionGroup(batch llama.BatchExt, index, group int32) error {
	if batchExtSetDecisionGroupFunc.Cif == nil {
		return fmt.Errorf("decision group binding is unavailable")
	}
	var result ffi.Arg
	batchExtSetDecisionGroupFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&batch), &index, &group)
	if !result.Bool() {
		return fmt.Errorf("%w: set decision group %d at index %d", llama.ErrBatchExtRejected, group, index)
	}
	return nil
}
