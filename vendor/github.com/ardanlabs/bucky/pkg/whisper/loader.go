package whisper

import (
	"fmt"

	"github.com/ardanlabs/bucky/pkg/loader"
)

var libPath string

// LibPath returns the path to the loaded whisper.cpp shared library.
func LibPath() string {
	return libPath
}

// Load loads the shared whisper.cpp library from the specified path and
// resolves all FFI function pointers used by this package.
//
// Load does NOT register ggml backends. Call Init after Load (and before
// the first model load) to populate the process-wide ggml backend
// registry from the same directory.
//
// The whisper.cpp xcframework on darwin bundles ggml inside libwhisper, so a
// single library load is sufficient. On windows, ggml-base/ggml-cpu/ggml DLLs
// live next to whisper.dll and the OS resolves them automatically when
// libwhisper is loaded.
func Load(path string) error {
	libPath = path

	lib, err := loader.LoadLibrary(path, "whisper")
	if err != nil {
		return err
	}

	if err := loadSystemFuncs(lib, path); err != nil {
		return err
	}

	if err := loadLogFuncs(lib); err != nil {
		return err
	}

	if err := loadContextFuncs(lib); err != nil {
		return err
	}

	if err := loadModelFuncs(lib); err != nil {
		return err
	}

	if err := loadParamsFuncs(lib); err != nil {
		return err
	}

	if err := loadFullFuncs(lib); err != nil {
		return err
	}

	if err := loadSegmentsFuncs(lib); err != nil {
		return err
	}

	if err := loadTokensFuncs(lib); err != nil {
		return err
	}

	if err := loadLangFuncs(lib); err != nil {
		return err
	}

	if err := loadStateFuncs(lib); err != nil {
		return err
	}

	if err := loadVadFuncs(lib); err != nil {
		return err
	}

	if err := loadBenchFuncs(lib); err != nil {
		return err
	}

	return nil
}

// Init registers every ggml backend shared library found under path with
// the process-wide ggml registry. Required when libwhisper was built with
// -DGGML_BACKEND_DL=ON (e.g. bucky-builder's Linux artifacts), where
// backends ship as separate libggml-*.so files that don't auto-register on
// libwhisper load. No-op on static builds (the upstream macOS xcframework
// and Windows zip) because the underlying ggml symbol is not exported.
//
// Call Init AFTER Load and BEFORE the first model load.
//
// Callers running whisper alongside another ggml-based library (e.g.
// llama.cpp via yzma) that already populated the registry SHOULD skip
// this call to avoid registering the same physical device twice. Use
// llama.GGMLBackendDeviceCount() > 0 as the sentinel in that case.
func Init(path string) error {
	if err := ggmlBackendLoadAllFromPath(path); err != nil {
		return fmt.Errorf("ggml_backend_load_all_from_path: %w", err)
	}

	return nil
}

func loadError(name string, err error) error {
	return fmt.Errorf("could not load %q: %w", name, err)
}
