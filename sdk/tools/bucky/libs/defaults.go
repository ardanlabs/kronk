package libs

import (
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// Path returns the directory the runtime should load whisper.cpp
// libraries from. Resolution mirrors the rules used by New (see
// WithLibPath):
//
//  1. An explicit override or KRONK_BUCKY_LIB_PATH that already
//     contains a version.json (or any non-empty user-managed
//     directory) is returned as-is.
//  2. Otherwise the path is the per-triple install directory under the
//     libraries root: <root>/<os>/<arch>/<processor>/.
//
// If resolution fails (very rare — only for unparseable env values),
// the legacy root is returned so callers see a clear "library not
// found" error rather than a confusing path-resolution failure.
func Path(override string) string {
	if override == "" {
		override = os.Getenv("KRONK_BUCKY_LIB_PATH")
	}

	lib, err := New(WithLibPath(override))
	if err != nil {
		if override != "" {
			return override
		}
		if v := os.Getenv("KRONK_BUCKY_LIB_PATH"); v != "" {
			return v
		}
		return filepath.Join(defaults.BaseDir(""), localFolder)
	}
	return lib.LibsPath()
}
