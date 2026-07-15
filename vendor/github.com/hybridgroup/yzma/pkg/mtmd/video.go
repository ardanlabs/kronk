package mtmd

import (
	"os"
	"unsafe"

	"github.com/hybridgroup/yzma/pkg/utils"
	"github.com/jupiterrider/ffi"
)

// VideoInfo contains metadata about a video file.
type VideoInfo struct {
	Width   uint32
	Height  uint32
	FPS     float32
	NFrames int32 // estimated total frames at the effective fps; -1 if unknown
}

// VideoInitParams contains parameters for initializing a VideoContext.
// Use VideoInitParamsDefault to obtain a copy with sensible defaults.
//
// Memory layout mirrors the C struct mtmd_helper_video_init_params:
//
//	float fps_target           (4 bytes, offset  0)
//	<4 bytes implicit padding>
//	const char * ffmpeg_bin_dir (8 bytes, offset  8)
//	int64_t timestamp_interval  (8 bytes, offset 16)
type VideoInitParams struct {
	// FPSTarget is the desired output fps. A value <= 0 uses the video's native fps.
	FPSTarget float32
	// FFmpegBinDir is the directory containing ffmpeg/ffprobe binaries.
	// nil means search the system PATH.
	// Use utils.BytePtrFromString to convert a Go string to the required *byte.
	FFmpegBinDir *byte
	// TimestampIntervalMs is the interval in milliseconds between timestamp text
	// chunks inserted into the token stream (e.g. "[10m50.5s]").
	// A value <= 0 disables timestamps.
	TimestampIntervalMs int64
}

var (
	// ffiTypeVideoInfo mirrors struct mtmd_helper_video_info
	ffiTypeVideoInfo = ffi.NewType(&ffi.TypeUint32, &ffi.TypeUint32, &ffi.TypeFloat, &ffi.TypeSint32)

	// ffiTypeVideoInitParams mirrors struct mtmd_helper_video_init_params
	ffiTypeVideoInitParams = ffi.NewType(&ffi.TypeFloat, &ffi.TypePointer, &ffi.TypeSint64)
)

var (
	// MTMD_API bool mtmd_helper_support_video(mtmd_context * ctx);
	helperSupportVideoFunc ffi.Fun

	// MTMD_API struct mtmd_helper_video_init_params mtmd_helper_video_init_params_default(void);
	helperVideoInitParamsDefaultFunc ffi.Fun

	// MTMD_API mtmd_helper_video * mtmd_helper_video_init(
	//     struct mtmd_context * mctx,
	//     const char * path,
	//     struct mtmd_helper_video_init_params params);
	helperVideoInitFunc ffi.Fun

	// MTMD_API mtmd_helper_video * mtmd_helper_video_init_from_buf(
	//     struct mtmd_context * mctx,
	//     const unsigned char * buf, size_t len,
	//     struct mtmd_helper_video_init_params params);
	helperVideoInitFromBufFunc ffi.Fun

	// MTMD_API void mtmd_helper_video_free(mtmd_helper_video * ctx);
	helperVideoFreeFunc ffi.Fun

	// MTMD_API struct mtmd_helper_video_info mtmd_helper_video_get_info(const mtmd_helper_video * ctx);
	helperVideoGetInfoFunc ffi.Fun
)

func loadVideoFuncs(lib ffi.Lib) error {
	var err error

	if helperSupportVideoFunc, err = lib.Prep("mtmd_helper_support_video", &ffi.TypeUint8, &ffi.TypePointer); err != nil {
		return loadError("mtmd_helper_support_video", err)
	}

	if helperVideoInitParamsDefaultFunc, err = lib.Prep("mtmd_helper_video_init_params_default", &ffiTypeVideoInitParams); err != nil {
		return loadError("mtmd_helper_video_init_params_default", err)
	}

	if helperVideoInitFunc, err = lib.Prep("mtmd_helper_video_init", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeVideoInitParams); err != nil {
		return loadError("mtmd_helper_video_init", err)
	}

	if helperVideoInitFromBufFunc, err = lib.Prep("mtmd_helper_video_init_from_buf", &ffi.TypePointer, &ffi.TypePointer, &ffi.TypePointer, &ffiTypeSize, &ffiTypeVideoInitParams); err != nil {
		return loadError("mtmd_helper_video_init_from_buf", err)
	}

	if helperVideoFreeFunc, err = lib.Prep("mtmd_helper_video_free", &ffi.TypeVoid, &ffi.TypePointer); err != nil {
		return loadError("mtmd_helper_video_free", err)
	}

	if helperVideoGetInfoFunc, err = lib.Prep("mtmd_helper_video_get_info", &ffiTypeVideoInfo, &ffi.TypePointer); err != nil {
		return loadError("mtmd_helper_video_get_info", err)
	}

	return nil
}

// SupportVideo reports whether the build of libmtmd includes video support
// (i.e. was compiled with MTMD_VIDEO). Requires ffmpeg/ffprobe on the system PATH at runtime.
func SupportVideo(ctx Context) bool {
	if ctx == 0 {
		return false
	}
	var result ffi.Arg
	helperSupportVideoFunc.Call(unsafe.Pointer(&result), unsafe.Pointer(&ctx))
	return result.Bool()
}

// VideoInitParamsDefault returns default video initialization parameters:
// fps_target=4.0, ffmpeg_bin_dir=nil (search PATH), timestamp_interval_ms=5000.
func VideoInitParamsDefault() VideoInitParams {
	var params VideoInitParams
	helperVideoInitParamsDefaultFunc.Call(unsafe.Pointer(&params))
	return params
}

// VideoInit initializes a VideoContext from a video file on disk.
// Returns 0 on failure (e.g. file not found, ffprobe not available).
// Requires ffmpeg and ffprobe to be installed on the system.
//
// The caller must call VideoFree when done.
//
// To use a specific ffmpeg directory, set params.FFmpegBinDir:
//
//	params := mtmd.VideoInitParamsDefault()
//	params.FFmpegBinDir, _ = utils.BytePtrFromString("/usr/local/bin")
func VideoInit(ctx Context, path string, params VideoInitParams) VideoContext {
	var videoCtx VideoContext
	if ctx == 0 {
		return videoCtx
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return videoCtx
	}
	pathPtr, _ := utils.BytePtrFromString(path)
	helperVideoInitFunc.Call(unsafe.Pointer(&videoCtx), unsafe.Pointer(&ctx), unsafe.Pointer(&pathPtr), unsafe.Pointer(&params))
	return videoCtx
}

// VideoInitFromBuf initializes a VideoContext from an in-memory video buffer.
// The buffer is copied internally; it does not need to remain live after this call.
// Returns 0 on failure.
//
// The caller must call VideoFree when done.
func VideoInitFromBuf(ctx Context, buf []byte, params VideoInitParams) VideoContext {
	var videoCtx VideoContext
	if ctx == 0 || len(buf) == 0 {
		return videoCtx
	}
	bufPtr := unsafe.SliceData(buf)
	bufLen := uint64(len(buf))
	helperVideoInitFromBufFunc.Call(unsafe.Pointer(&videoCtx), unsafe.Pointer(&ctx), unsafe.Pointer(&bufPtr), &bufLen, unsafe.Pointer(&params))
	return videoCtx
}

// VideoFree frees a VideoContext previously returned by VideoInit or VideoInitFromBuf.
// It must remain alive until after any Tokenize call that uses the associated lazy bitmap.
func VideoFree(videoCtx VideoContext) {
	if videoCtx == 0 {
		return
	}
	helperVideoFreeFunc.Call(nil, unsafe.Pointer(&videoCtx))
}

// VideoGetInfo returns metadata (dimensions, fps, frame count) for the video.
func VideoGetInfo(videoCtx VideoContext) VideoInfo {
	var info VideoInfo
	if videoCtx == 0 {
		return info
	}
	helperVideoGetInfoFunc.Call(unsafe.Pointer(&info), unsafe.Pointer(&videoCtx))
	return info
}
