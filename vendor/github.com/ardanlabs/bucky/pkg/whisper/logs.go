package whisper

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/jupiterrider/ffi"
)

// LogCallback is the type for a logging callback function pointer
// installed via LogSet. The value is the raw C function pointer the
// FFI layer will register with whisper.cpp / ggml.
type LogCallback uintptr

var (
	// WHISPER_API void whisper_log_set(ggml_log_callback log_callback, void * user_data);
	logSetFunc ffi.Fun

	// GGML_API void ggml_log_set(ggml_log_callback log_callback, void * user_data);
	//
	// Loaded best-effort. On builds where ggml is statically linked
	// inside libwhisper (the macOS xcframework, the upstream Windows
	// zip) the symbol may not be re-exported by libwhisper. We treat
	// a Prep failure as a soft no-op; the whisper-side callback
	// already covers the higher-level log lines, while the deeper
	// ggml backend chatter falls through to stderr only on those
	// builds.
	ggmlLogSetFunc ffi.Fun
)

func loadLogFuncs(lib ffi.Lib) error {
	var err error

	if logSetFunc, err = lib.Prep("whisper_log_set", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("whisper_log_set", err)
	}

	if fn, perr := lib.Prep("ggml_log_set", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); perr == nil {
		ggmlLogSetFunc = fn
	}

	return nil
}

// LogSet installs cb as the active whisper.cpp / ggml log callback.
// Pass LogSilent() to suppress all C-side logging. Pass LogNormal
// to restore whisper.cpp's default stderr printer.
//
// When ggml_log_set is available on the loaded library (most
// non-static builds), it is configured with the same callback so the
// backend lines (ggml_metal_*, ggml_backend_*, …) follow the same
// policy.
func LogSet(cb uintptr) {
	nada := uintptr(0)
	logSetFunc.Call(nil, unsafe.Pointer(&cb), unsafe.Pointer(&nada))

	if ggmlLogSetFunc != (ffi.Fun{}) {
		ggmlLogSetFunc.Call(nil, unsafe.Pointer(&cb), unsafe.Pointer(&nada))
	}
}

// LogSilent returns a callback function pointer that you can pass
// into LogSet to suppress all C-side logging.
func LogSilent() uintptr {
	return purego.NewCallback(func(level int32, text, data uintptr) uintptr {
		return 0
	})
}

// LogNormal is the value you can pass into LogSet to restore the
// default whisper.cpp / ggml logging behavior (writes to stderr).
const LogNormal uintptr = 0
