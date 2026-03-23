package llama

import (
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/jupiterrider/ffi"
)

// LogCallback is a type for the logging callback function.
type LogCallback uintptr

var (
	// LLAMA_API void llama_log_set(ggml_log_callback log_callback, void * user_data);
	logSetFunc ffi.Fun

	// LLAMA_API void llama_log_get(ggml_log_callback * log_callback, void ** user_data);
	logGetFunc ffi.Fun
)

func loadLogFuncs(lib ffi.Lib) error {
	var err error

	if logSetFunc, err = lib.Prep("llama_log_set", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_log_set", err)
	}

	if logGetFunc, err = lib.Prep("llama_log_get", &ffi.TypeVoid, &ffi.TypePointer, &ffi.TypePointer); err != nil {
		return loadError("llama_log_get", err)
	}

	return nil
}

// LogSet sets the logging mode. Pass llama.LogSilent() to turn logging off. Pass nil to use stdout.
func LogSet(cb uintptr) {
	nada := uintptr(0)
	logSetFunc.Call(nil, unsafe.Pointer(&cb), unsafe.Pointer(&nada))
}

// LogGet retrieves the current log callback and user data.
// Returns the callback function pointer and the user data pointer.
// LogSet must be called before LogGet, otherwise the C function
// may dereference a NULL internal pointer.
func LogGet() (uintptr, uintptr) {
	var cb uintptr
	var ud uintptr
	cbPtr := unsafe.Pointer(&cb)
	udPtr := unsafe.Pointer(&ud)
	logGetFunc.Call(nil, unsafe.Pointer(&cbPtr), unsafe.Pointer(&udPtr))
	return cb, ud
}

// LogSilent is a callback function that you can pass into the LogSet function to turn logging off.
func LogSilent() uintptr {
	return purego.NewCallback(func(level int32, text, data uintptr) uintptr {
		return 0
	})
}

// LogNormal is a value you can pass into the LogSet function to turn standard logging on.
const LogNormal uintptr = 0
