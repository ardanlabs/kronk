package whisper

import (
	"errors"
	"fmt"
	"unsafe"

	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API int whisper_full(
	//             struct whisper_context * ctx,
	//         struct whisper_full_params   params,
	//                        const float * samples,
	//                                int   n_samples);
	fullFunc ffi.Fun

	// WHISPER_API int whisper_full_parallel(
	//             struct whisper_context * ctx,
	//         struct whisper_full_params   params,
	//                        const float * samples,
	//                                int   n_samples,
	//                                int   n_processors);
	fullParallelFunc ffi.Fun
)

func loadFullFuncs(lib ffi.Lib) error {
	var err error

	if fullFunc, err = lib.Prep("whisper_full",
		&ffi.TypeSint32,
		&ffi.TypePointer,
		&ffiTypeFullParams,
		&ffi.TypePointer,
		&ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full", err)
	}

	if fullParallelFunc, err = lib.Prep("whisper_full_parallel",
		&ffi.TypeSint32,
		&ffi.TypePointer,
		&ffiTypeFullParams,
		&ffi.TypePointer,
		&ffi.TypeSint32,
		&ffi.TypeSint32,
	); err != nil {
		return loadError("whisper_full_parallel", err)
	}

	return nil
}

// Full runs the full whisper pipeline (mel -> encoder -> decoder) on the
// provided 16 kHz mono float32 PCM samples. It is a blocking call.
//
// Returns nil on success, or an error containing the non-zero return code
// from whisper.cpp on failure.
func Full(ctx Context, params WhisperFullParams, samples []float32) error {
	if ctx == 0 {
		return errors.New("whisper.Full: nil context")
	}
	if len(samples) == 0 {
		return errors.New("whisper.Full: empty samples")
	}

	samplesPtr := unsafe.Pointer(unsafe.SliceData(samples))
	nSamples := int32(len(samples))

	var result ffi.Arg
	fullFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&params),
		unsafe.Pointer(&samplesPtr),
		unsafe.Pointer(&nSamples),
	)

	if rc := int32(result); rc != 0 {
		return fmt.Errorf("whisper_full returned %d", rc)
	}
	return nil
}

// FullParallel splits the input audio into nProcessors chunks and processes
// each in parallel using whisper_full_with_state internally. Faster on long
// clips at some cost in transcription accuracy at chunk boundaries.
func FullParallel(ctx Context, params WhisperFullParams, samples []float32, nProcessors int32) error {
	if ctx == 0 {
		return errors.New("whisper.FullParallel: nil context")
	}
	if len(samples) == 0 {
		return errors.New("whisper.FullParallel: empty samples")
	}
	if nProcessors < 1 {
		nProcessors = 1
	}

	samplesPtr := unsafe.Pointer(unsafe.SliceData(samples))
	nSamples := int32(len(samples))

	var result ffi.Arg
	fullParallelFunc.Call(
		unsafe.Pointer(&result),
		unsafe.Pointer(&ctx),
		unsafe.Pointer(&params),
		unsafe.Pointer(&samplesPtr),
		unsafe.Pointer(&nSamples),
		unsafe.Pointer(&nProcessors),
	)

	if rc := int32(result); rc != 0 {
		return fmt.Errorf("whisper_full_parallel returned %d", rc)
	}
	return nil
}
