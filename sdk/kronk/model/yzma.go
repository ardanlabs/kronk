// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.

package model

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/jupiterrider/ffi"
)

// MTMDHelperCapabilities describes native model and projector capabilities, not
// features implemented by Kronk. It mirrors struct mtmd_helper_cap in field order.
type MTMDHelperCapabilities struct {
	Chat           bool
	Vision         bool
	Audio          bool
	Video          bool
	AudioStream    bool
	TextGen        bool
	AudioGen       bool
	AudioGenSpkRef bool
}

// MTMDAudioStream is an opaque native mtmd_audio_stream pointer. Its owner must
// keep the stream alive while calling the row getters below.
type MTMDAudioStream uintptr

var (
	mtmdHelperCapabilitiesType = ffi.NewType(
		&ffi.TypeUint8, // chat
		&ffi.TypeUint8, // vision
		&ffi.TypeUint8, // audio
		&ffi.TypeUint8, // video
		&ffi.TypeUint8, // audio_stream
		&ffi.TypeUint8, // text_gen
		&ffi.TypeUint8, // audio_gen
		&ffi.TypeUint8, // audio_gen_spk_ref
	)

	mtmdHelperGetCapabilitiesFunc      ffi.Fun
	mtmdAudioStreamGetNRowsFunc        ffi.Fun
	mtmdAudioStreamGetNRowsFlushedFunc ffi.Fun
)

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
// Call it during native library initialization, before using these bindings.
func InitYzmaWorkarounds(path string) error {
	lib, err := loader.LoadLibrary(path, "mtmd")
	if err != nil {
		return fmt.Errorf("loading mtmd compatibility library: %w", err)
	}

	// These additive APIs are optional so older native bundles still load.
	// Reset each binding before probing in case initialization is repeated.
	mtmdHelperGetCapabilitiesFunc = ffi.Fun{}
	mtmdAudioStreamGetNRowsFunc = ffi.Fun{}
	mtmdAudioStreamGetNRowsFlushedFunc = ffi.Fun{}

	if fn, err := lib.Prep("mtmd_helper_get_capabilities", &mtmdHelperCapabilitiesType, &ffi.TypePointer, &ffi.TypePointer); err == nil {
		mtmdHelperGetCapabilitiesFunc = fn
	}
	if fn, err := lib.Prep("mtmd_audio_stream_get_n_rows", &ffi.TypeSint64, &ffi.TypePointer); err == nil {
		mtmdAudioStreamGetNRowsFunc = fn
	}
	if fn, err := lib.Prep("mtmd_audio_stream_get_n_rows_flushed", &ffi.TypeSint64, &ffi.TypePointer); err == nil {
		mtmdAudioStreamGetNRowsFlushedFunc = fn
	}

	return nil
}

// MTMDHelperGetCapabilities returns native capabilities for a loaded model and
// projector. The projector may be zero for a text-only model. The handles must
// remain loaded during the call. An error means the binding is unavailable.
func MTMDHelperGetCapabilities(model llama.Model, ctx mtmd.Context) (MTMDHelperCapabilities, error) {
	if mtmdHelperGetCapabilitiesFunc.Addr == 0 {
		return MTMDHelperCapabilities{}, errors.New("mtmd_helper_get_capabilities: binding unavailable")
	}

	var capabilities MTMDHelperCapabilities
	mtmdHelperGetCapabilitiesFunc.Call(unsafe.Pointer(&capabilities), &model, &ctx)
	return capabilities, nil
}

// MTMDAudioStreamGetNRows returns the rows produced so far, including prefix
// rows. The stream must be a nonzero, live native stream. An error means the
// binding is unavailable.
func MTMDAudioStreamGetNRows(stream MTMDAudioStream) (int64, error) {
	if mtmdAudioStreamGetNRowsFunc.Addr == 0 {
		return 0, errors.New("mtmd_audio_stream_get_n_rows: binding unavailable")
	}

	var rows int64
	mtmdAudioStreamGetNRowsFunc.Call(&rows, &stream)
	return rows, nil
}

// MTMDAudioStreamGetNRowsFlushed returns the row endpoint of the last flush.
// Once produced rows reach this endpoint, all flushed audio rows have been
// produced, not necessarily decoded or delivered. The stream must be nonzero
// and live. An error means the binding is unavailable.
func MTMDAudioStreamGetNRowsFlushed(stream MTMDAudioStream) (int64, error) {
	if mtmdAudioStreamGetNRowsFlushedFunc.Addr == 0 {
		return 0, errors.New("mtmd_audio_stream_get_n_rows_flushed: binding unavailable")
	}

	var rows int64
	mtmdAudioStreamGetNRowsFlushedFunc.Call(&rows, &stream)
	return rows, nil
}
