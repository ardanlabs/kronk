// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.

package model

import (
	"fmt"
	"strconv"
	"strings"
	"unsafe"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/jupiterrider/ffi"
)

// Temporary binding until https://github.com/hybridgroup/yzma/issues/508 lands.
var batchExtSetDecisionGroupFunc ffi.Fun

// Temporary binding until https://github.com/hybridgroup/yzma/issues/509 lands.
// b12619 inserted audio_duplex before text_gen, changing the struct-return ABI.
var (
	ffiTypeHelperCap = ffi.NewType(&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8,
		&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8)
	ffiTypeHelperCapDuplex = ffi.NewType(&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8,
		&ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8, &ffi.TypeUint8,
		&ffi.TypeUint8, &ffi.TypeUint8)
	helperGetCapabilitiesFunc ffi.Fun
	helperCapDuplex           bool
)

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

	helperGetCapabilitiesFunc = ffi.Fun{}
	mtmdLib, err := loader.LoadLibrary(path, "mtmd")
	if err != nil {
		return fmt.Errorf("load capability binding: %w", err)
	}
	capFunc, err := mtmdLib.Prep("mtmd_helper_get_capabilities", &ffiTypeHelperCapDuplex, &ffi.TypePointer, &ffi.TypePointer)
	if err != nil {
		// Preserve the fallback for bundles predating capability discovery.
		return nil
	}
	tag, err := libs.ReadVersionFile(loader.ResolvePath(path))
	if err != nil {
		return fmt.Errorf("select capability ABI: %w", err)
	}
	version, _, _ := strings.Cut(tag.Version, "@")
	build, err := strconv.Atoi(strings.TrimPrefix(version, "b"))
	if err != nil || !strings.HasPrefix(version, "b") || build <= 0 {
		return fmt.Errorf("select capability ABI: unrecognized llama.cpp version %q", tag.Version)
	}
	helperCapDuplex = build >= 12619
	if !helperCapDuplex {
		capFunc, err = mtmdLib.Prep("mtmd_helper_get_capabilities", &ffiTypeHelperCap, &ffi.TypePointer, &ffi.TypePointer)
		if err != nil {
			return fmt.Errorf("prepare capability binding: %w", err)
		}
	}
	helperGetCapabilitiesFunc = capFunc
	return nil
}

func helperGetCapabilities(model llama.Model, ctx mtmd.Context) (mtmd.HelperCap, error) {
	if helperGetCapabilitiesFunc.Cif == nil {
		return mtmd.HelperCap{}, mtmd.ErrNoCapabilities
	}
	if model == 0 {
		return mtmd.HelperCap{}, nil
	}
	if !helperCapDuplex {
		var result mtmd.HelperCap
		helperGetCapabilitiesFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model), unsafe.Pointer(&ctx))
		return result, nil
	}
	// Do not expose duplex support: Kronk does not implement a duplex runtime.
	var result struct {
		Chat, Vision, Audio, Video, AudioStream, AudioDuplex bool
		TextGen, AudioGen, AudioGenSpkRef                    bool
	}
	helperGetCapabilitiesFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&model), unsafe.Pointer(&ctx))
	return mtmd.HelperCap{
		Chat: result.Chat, Vision: result.Vision, Audio: result.Audio,
		Video: result.Video, AudioStream: result.AudioStream,
		TextGen: result.TextGen, AudioGen: result.AudioGen, AudioGenSpkRef: result.AudioGenSpkRef,
	}, nil
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
