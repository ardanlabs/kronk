package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/jupiterrider/ffi"
)

// LoadLibrary The path can be an empty string to use the location as set by the YZMA_LIB env variable.
// The lib should be the "short name" for the library, for example:
// gguf, llama, mtmd
func LoadLibrary(path, lib string) (ffi.Lib, error) {
	if path == "" && os.Getenv("YZMA_LIB") != "" {
		path = os.Getenv("YZMA_LIB")
	}

	// Ensure the library path is set
	if path == "" {
		return ffi.Lib{}, fmt.Errorf("library path not specified and YZMA_LIB env variable not set")
	}

	filename := GetLibraryFilename(path, lib)

	return ffi.Load(filename)
}

// GetLibraryFilename returns the full path to the library file for the given path and library name.
// The library name should be the "short name" (e.g., "llama", "gguf", "mtmd").
// The function returns the appropriate filename based on the current OS:
//   - Linux/FreeBSD: lib<name>.so
//   - Windows: <name>.dll
//   - Darwin: lib<name>.dylib
func GetLibraryFilename(path, lib string) string {
	switch runtime.GOOS {
	case "linux", "freebsd":
		return filepath.Join(path, fmt.Sprintf("lib%s.so", lib))
	case "windows":
		return filepath.Join(path, fmt.Sprintf("%s.dll", lib))
	case "darwin":
		return filepath.Join(path, fmt.Sprintf("lib%s.dylib", lib))
	default:
		return filepath.Join(path, lib)
	}
}
