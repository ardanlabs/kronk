// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.

package model

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// ErrModelDecisionLabelUnavailable means the loaded llama.cpp library does not
// export llama_model_decision_label.
var ErrModelDecisionLabelUnavailable = errors.New("llama_model_decision_label not found in library")

var modelDecisionLabelFunc ffi.Fun

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
func InitYzmaWorkarounds(path string) error {
	lib, err := loader.LoadLibrary(path, "llama")
	if err != nil {
		return fmt.Errorf("loading llama compatibility library: %w", err)
	}

	// This additive API is optional so older llama.cpp bundles still load.
	modelDecisionLabelFunc = ffi.Fun{}
	// Prefer C linkage when upstream adds it; b11797 uses C++ linkage.
	for _, name := range []string{
		"llama_model_decision_label",
		"_Z26llama_model_decision_labelPK11llama_modelj",          // Itanium ABI (Clang/GCC).
		"?llama_model_decision_label@@YAPEBDPEBUllama_model@@I@Z", // MSVC ABI (64-bit).
	} {
		if fn, err := lib.Prep(name, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypeUint32); err == nil {
			modelDecisionLabelFunc = fn
			break
		}
	}

	return nil
}

// ModelDecisionLabel returns a Go-owned copy of the model's decision label at
// index, preserving whitespace. An empty string and nil error indicate a null
// model or no label at that index. The model must remain loaded during the call.
// C and C++ exports (Itanium and 64-bit MSVC ABIs) are supported.
// ErrModelDecisionLabelUnavailable indicates a library without a supported export.
func ModelDecisionLabel(model llama.Model, index uint32) (string, error) {
	if modelDecisionLabelFunc.Addr == 0 {
		return "", ErrModelDecisionLabelUnavailable
	}
	if model == 0 {
		return "", nil
	}

	var labelPtr *byte
	modelDecisionLabelFunc.Call(unsafe.Pointer(&labelPtr), unsafe.Pointer(&model), &index)
	if labelPtr == nil {
		return "", nil
	}

	return utils.BytePtrToString(labelPtr), nil
}
