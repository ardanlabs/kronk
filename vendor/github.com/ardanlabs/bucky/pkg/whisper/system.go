package whisper

import (
	"unsafe"

	"github.com/ardanlabs/bucky/pkg/loader"
	"github.com/ardanlabs/bucky/pkg/utils"
	"github.com/jupiterrider/ffi"
)

var (
	// WHISPER_API const char * whisper_version(void);
	versionFunc ffi.Fun

	// WHISPER_API const char * whisper_print_system_info(void);
	printSystemInfoFunc ffi.Fun

	// GGML_API void ggml_backend_load_all_from_path(const char * dir_path);
	//
	// Resolution differs per platform:
	//   - Linux: libwhisper.so re-exports symbols from libggml-base.so via
	//     the global dynamic symbol namespace, so dlsym on libwhisper
	//     finds it.
	//   - Windows: GetProcAddress is strict per-DLL. whisper.dll exports
	//     the ggml_* symbols it uses internally but NOT this one (apps
	//     are expected to call it), so we look it up in ggml.dll as a
	//     fallback. Without this fallback Init() silently no-ops and
	//     ggml ends up with zero registered backends.
	//   - Darwin xcframework: backends are statically linked into the
	//     framework binary and the symbol is not exported at all; the
	//     loader treats a missing symbol as a soft no-op.
	ggmlBackendLoadAllFromPathFunc ffi.Fun
)

func loadSystemFuncs(lib ffi.Lib, path string) error {
	var err error

	if versionFunc, err = lib.Prep("whisper_version", &ffi.TypePointer); err != nil {
		return loadError("whisper_version", err)
	}

	if printSystemInfoFunc, err = lib.Prep("whisper_print_system_info", &ffi.TypePointer); err != nil {
		return loadError("whisper_print_system_info", err)
	}

	// Try whisper first. On Linux this works because libwhisper.so
	// re-exports libggml-base.so's symbols. On the upstream darwin
	// xcframework and on builds without GGML_BACKEND_DL=ON, the symbol
	// is absent entirely and this fails benignly.
	if fn, perr := lib.Prep("ggml_backend_load_all_from_path", &ffi.TypeVoid, &ffi.TypePointer); perr == nil {
		ggmlBackendLoadAllFromPathFunc = fn
		return nil
	}

	// Fallback: on Windows the symbol lives in ggml.dll, not whisper
	// .dll. Open ggml.dll separately and look it up there. A failure
	// here is non-fatal — it just means we'll skip ggml_backend_load
	// _all_from_path in Init().
	ggmlLib, perr := loader.LoadLibrary(path, "ggml")
	if perr != nil {
		return nil
	}
	if fn, perr := ggmlLib.Prep("ggml_backend_load_all_from_path", &ffi.TypeVoid, &ffi.TypePointer); perr == nil {
		ggmlBackendLoadAllFromPathFunc = fn
	}

	return nil
}

// Version returns the whisper.cpp library version string.
func Version() string {
	var ptr *byte
	versionFunc.Call(unsafe.Pointer(&ptr))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// PrintSystemInfo returns the system info string reported by whisper.cpp
// (the same string the upstream CLI prints at startup).
func PrintSystemInfo() string {
	var ptr *byte
	printSystemInfoFunc.Call(unsafe.Pointer(&ptr))
	if ptr == nil {
		return ""
	}
	return utils.BytePtrToString(ptr)
}

// ggmlBackendLoadAllFromPath dlopens every libggml-*.{so,dylib,dll} found in
// dirPath so each backend's static constructor self-registers with ggml.
//
// Required for builds where ggml backends ship as separate dynamic
// libraries (-DGGML_BACKEND_DL=ON), which is how bucky-builder produces the
// Linux artifacts. Without this call, ggml ends up with zero registered
// backends and the first model init asserts on device==NULL.
//
// On builds where backends are statically linked into libwhisper (the
// upstream macOS xcframework, the upstream Windows zip), the symbol is not
// present at all and this is a no-op (loadSystemFuncs leaves the Fun
// zero-valued). On builds where the symbol is present but no
// libggml-*.{so,dylib,dll} files match in dirPath, the underlying ggml
// implementation is itself a safe no-op.
func ggmlBackendLoadAllFromPath(dirPath string) error {
	if ggmlBackendLoadAllFromPathFunc == (ffi.Fun{}) {
		return nil
	}
	cpath, err := utils.BytePtrFromString(dirPath)
	if err != nil {
		return err
	}
	ggmlBackendLoadAllFromPathFunc.Call(nil, unsafe.Pointer(&cpath))
	return nil
}
