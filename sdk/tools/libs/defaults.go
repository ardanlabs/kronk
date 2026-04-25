package libs

import (
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// Path returns the directory the runtime should load llama.cpp libraries
// from. Resolution mirrors the rules used by New (see WithLibPath):
//
//  1. An explicit override or KRONK_LIB_PATH that already contains a
//     version.json (or any non-empty user-managed directory) is returned
//     as-is.
//  2. Otherwise the path is the per-triple install directory under the
//     libraries root: <root>/<os>/<arch>/<processor>/.
//
// Path also performs the legacy-layout migration on first call so that
// existing installs at <root>/*.so are moved into their triple folder
// before the runtime tries to load from them.
func Path(override string) string {
	if override == "" {
		override = os.Getenv("KRONK_LIB_PATH")
	}

	lib, err := New(WithLibPath(override))
	if err != nil {
		// Resolution failed (very rare — only for unparseable env values).
		// Fall back to the legacy root so callers see a clear "library not
		// found" error rather than a confusing path-resolution failure.
		if override != "" {
			return override
		}
		if v := os.Getenv("KRONK_LIB_PATH"); v != "" {
			return v
		}
		return filepath.Join(defaults.BaseDir(""), localFolder)
	}
	return lib.LibsPath()
}
