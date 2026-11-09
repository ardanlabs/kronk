// DO NOT DELETE THIS FILE when InitYzmaWorkarounds is empty.
// Pending upstream speech bindings: https://github.com/hybridgroup/yzma/issues/485

package model

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/loader"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// MTMDHelperGenAudio is an opaque native audio pipeline. Calls sharing either
// context must be serialized. Free the helper before freeing its contexts.
type MTMDHelperGenAudio uintptr

// MTMDHelperGenAudioInput mirrors mtmd_helper_gen_audio_inp in llama.cpp b12223.
// Prompt, Lang and SpeakerRef must remain valid until SetInput returns.
type MTMDHelperGenAudioInput struct {
	Stream     bool
	SeqID      llama.SeqId
	Prompt     *byte
	PromptLen  uintptr
	NPast      int32
	OutMode    int32
	SpeakerRef mtmd.Bitmap
	Lang       *byte
	TopK       int32
	TopP       float32
	Temp       float32
	Seed       uint32
	OutType    int32
}

// MTMDGenAudioOutputWAV selects PCM16 little-endian mono WAV output.
const MTMDGenAudioOutputWAV int32 = 1

var (
	mtmdGenAudioInputType        ffi.Type
	mtmdGenAudioAvailable        bool
	mtmdGenAudioInputDefaultFunc ffi.Fun
	mtmdGenAudioInitFunc         ffi.Fun
	mtmdGenAudioFreeFunc         ffi.Fun
	mtmdGenAudioResetFunc        ffi.Fun
	mtmdGenAudioSetInputFunc     ffi.Fun
	mtmdGenAudioStepPromptFunc   ffi.Fun
	mtmdGenAudioStepGenFunc      ffi.Fun
	mtmdGenAudioGetOutputFunc    ffi.Fun
	mtmdGenAudioFlushFunc        ffi.Fun
	mtmdGenAudioGetVoiceFunc     ffi.Fun
)

// InitYzmaWorkarounds initializes Kronk-specific Yzma compatibility code.
// Call it during native initialization, before using these bindings.
func InitYzmaWorkarounds(path string) error {
	lib, err := loader.LoadLibrary(path, "mtmd")
	if err != nil {
		return fmt.Errorf("loading mtmd compatibility library: %w", err)
	}
	mtmdGenAudioAvailable = false
	sizeType := &ffi.TypeUint64
	if unsafe.Sizeof(uintptr(0)) == 4 {
		sizeType = &ffi.TypeUint32
	}
	mtmdGenAudioInputType = ffi.NewType(
		&ffi.TypeUint8, &ffi.TypeSint32, &ffi.TypePointer, sizeType,
		&ffi.TypeSint32, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer,
		&ffi.TypeSint32, &ffi.TypeFloat, &ffi.TypeFloat, &ffi.TypeUint32, &ffi.TypeSint32,
	)
	functions := []struct {
		name   string
		target *ffi.Fun
		result *ffi.Type
		args   []*ffi.Type
	}{
		{"mtmd_helper_gen_audio_inp_default", &mtmdGenAudioInputDefaultFunc, &mtmdGenAudioInputType, nil},
		{"mtmd_helper_gen_audio_init", &mtmdGenAudioInitFunc, &ffi.TypePointer, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer}},
		{"mtmd_helper_gen_audio_free", &mtmdGenAudioFreeFunc, &ffi.TypeVoid, []*ffi.Type{&ffi.TypePointer}},
		{"mtmd_helper_gen_audio_reset", &mtmdGenAudioResetFunc, &ffi.TypeVoid, []*ffi.Type{&ffi.TypePointer}},
		{"mtmd_helper_gen_audio_set_input", &mtmdGenAudioSetInputFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer}},
		{"mtmd_helper_gen_audio_step_prompt", &mtmdGenAudioStepPromptFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32}},
		{"mtmd_helper_gen_audio_step_gen", &mtmdGenAudioStepGenFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer}},
		{"mtmd_helper_gen_audio_get_output", &mtmdGenAudioGetOutputFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer}},
		{"mtmd_helper_gen_audio_flush", &mtmdGenAudioFlushFunc, &ffi.TypeSint32, []*ffi.Type{&ffi.TypePointer}},
		{"mtmd_gen_audio_get_voice", &mtmdGenAudioGetVoiceFunc, &ffi.TypePointer, []*ffi.Type{&ffi.TypePointer, &ffi.TypePointer}},
	}
	for _, function := range functions {
		fn, err := lib.Prep(function.name, function.result, function.args...)
		if err != nil {
			return nil // Audio generation is optional on older native bundles.
		}
		*function.target = fn
	}
	mtmdGenAudioAvailable = true
	return nil
}

// MTMDHelperGenAudioInputDefault returns the native audio-generation defaults.
func MTMDHelperGenAudioInputDefault() (MTMDHelperGenAudioInput, error) {
	var input MTMDHelperGenAudioInput
	if !mtmdGenAudioAvailable {
		return input, ErrSpeechUnsupported
	}
	mtmdGenAudioInputDefaultFunc.Call(unsafe.Pointer(&input))
	return input, nil
}

// MTMDHelperGenAudioInit creates a pipeline borrowing the supplied contexts.
func MTMDHelperGenAudioInit(lctx llama.Context, mctx mtmd.Context) (MTMDHelperGenAudio, error) {
	var ctx MTMDHelperGenAudio
	if !mtmdGenAudioAvailable {
		return ctx, ErrSpeechUnsupported
	}
	if lctx == 0 || mctx == 0 {
		return ctx, errors.New("invalid audio generation context")
	}
	mtmdGenAudioInitFunc.Call(unsafe.Pointer(&ctx), &lctx, &mctx)
	if ctx == 0 {
		return ctx, errors.New("mtmd_helper_gen_audio_init failed")
	}
	return ctx, nil
}

// MTMDHelperGenAudioFree releases a pipeline, not its borrowed contexts.
func MTMDHelperGenAudioFree(ctx MTMDHelperGenAudio) {
	if mtmdGenAudioAvailable && ctx != 0 {
		mtmdGenAudioFreeFunc.Call(nil, &ctx)
	}
}

// MTMDHelperGenAudioReset discards pending output and pipeline state, not KV.
func MTMDHelperGenAudioReset(ctx MTMDHelperGenAudio) {
	if mtmdGenAudioAvailable && ctx != 0 {
		mtmdGenAudioResetFunc.Call(nil, &ctx)
	}
}

// MTMDHelperGenAudioSetInput prepares an utterance and its sequence.
func MTMDHelperGenAudioSetInput(ctx MTMDHelperGenAudio, input MTMDHelperGenAudioInput) error {
	if !mtmdGenAudioAvailable {
		return ErrSpeechUnsupported
	}
	if ctx == 0 {
		return errors.New("invalid audio generation handle")
	}
	p := &input
	var result ffi.Arg
	mtmdGenAudioSetInputFunc.Call(unsafe.Pointer(&result), &ctx, unsafe.Pointer(&p))
	runtime.KeepAlive(input)
	if int32(result) != 0 {
		return fmt.Errorf("mtmd_helper_gen_audio_set_input failed: %d", int32(result))
	}
	return nil
}

// MTMDHelperGenAudioStepPrompt decodes at most nBatch prompt rows. It returns
// the remaining count, zero when done, or a negative error code.
func MTMDHelperGenAudioStepPrompt(ctx MTMDHelperGenAudio, nBatch int32) int32 {
	if !mtmdGenAudioAvailable || ctx == 0 {
		return -1
	}
	var result ffi.Arg
	mtmdGenAudioStepPromptFunc.Call(unsafe.Pointer(&result), &ctx, &nBatch)
	return int32(result)
}

// MTMDHelperGenAudioStepGen advances one frame. Returned hidden state is
// borrowed until the next step or reset. stop marks end-of-speech.
func MTMDHelperGenAudioStepGen(ctx MTMDHelperGenAudio, token llama.Token, hidden *float32) (*float32, bool, error) {
	if !mtmdGenAudioAvailable {
		return nil, false, ErrSpeechUnsupported
	}
	if ctx == 0 {
		return nil, false, errors.New("invalid audio generation handle")
	}
	var next *float32
	var stop bool
	nextPtr, stopPtr := &next, &stop
	var result ffi.Arg
	mtmdGenAudioStepGenFunc.Call(unsafe.Pointer(&result), &ctx, &token, unsafe.Pointer(&hidden), unsafe.Pointer(&nextPtr), unsafe.Pointer(&stopPtr))
	runtime.KeepAlive(hidden)
	if int32(result) != 0 {
		return nil, false, fmt.Errorf("mtmd_helper_gen_audio_step_gen failed: %d", int32(result))
	}
	return next, stop, nil
}

// MTMDHelperGenAudioGetOutput copies native audio into Go-owned bytes and returns
// the sample rate and sample count. Streaming returns only newly produced audio.
func MTMDHelperGenAudioGetOutput(ctx MTMDHelperGenAudio) ([]byte, int32, int64, error) {
	if !mtmdGenAudioAvailable {
		return nil, 0, 0, ErrSpeechUnsupported
	}
	if ctx == 0 {
		return nil, 0, 0, errors.New("invalid audio generation handle")
	}
	var data *byte
	var size uintptr
	var rate int32
	var samples int64
	ratePtr, dataPtr, sizePtr, samplesPtr := &rate, &data, &size, &samples
	var result ffi.Arg
	mtmdGenAudioGetOutputFunc.Call(unsafe.Pointer(&result), &ctx, unsafe.Pointer(&ratePtr), unsafe.Pointer(&dataPtr), unsafe.Pointer(&sizePtr), unsafe.Pointer(&samplesPtr))
	if int32(result) != 0 {
		return nil, 0, 0, fmt.Errorf("mtmd_helper_gen_audio_get_output failed: %d", int32(result))
	}
	return slices.Clone(unsafe.Slice(data, size)), rate, samples, nil
}

// MTMDHelperGenAudioFlush decodes buffered frames before the final streaming
// output call. It must not be used to publish successfully truncated speech.
func MTMDHelperGenAudioFlush(ctx MTMDHelperGenAudio) error {
	if !mtmdGenAudioAvailable {
		return ErrSpeechUnsupported
	}
	if ctx == 0 {
		return errors.New("invalid audio generation handle")
	}
	var result ffi.Arg
	mtmdGenAudioFlushFunc.Call(unsafe.Pointer(&result), &ctx)
	if int32(result) != 0 {
		return fmt.Errorf("mtmd_helper_gen_audio_flush failed: %d", int32(result))
	}
	return nil
}

// MTMDGenAudioGetVoice creates a preset voice bitmap. The caller must free it
// with mtmd.BitmapFree after the helper has finished using it.
func MTMDGenAudioGetVoice(ctx mtmd.Context, name string) (mtmd.Bitmap, error) {
	if !mtmdGenAudioAvailable {
		return 0, ErrSpeechUnsupported
	}
	if ctx == 0 {
		return 0, errors.New("invalid mtmd context")
	}
	p, err := utils.BytePtrFromString(name)
	if err != nil {
		return 0, err
	}
	var voice mtmd.Bitmap
	mtmdGenAudioGetVoiceFunc.Call(unsafe.Pointer(&voice), &ctx, unsafe.Pointer(&p))
	runtime.KeepAlive(p)
	if voice == 0 {
		return 0, fmt.Errorf("unknown voice %q", name)
	}
	return voice, nil
}
