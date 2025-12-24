package libs

import (
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
)

// Path returns the location for the libraries folder. It will check the
// KRONK_LIB_PATH env var first and then default to the home directory if
// one can be identified. Last resort it will choose the current directory.
func Path(override string) string {
	if override != "" {
		return override
	}

	if v := os.Getenv("KRONK_LIB_PATH"); v != "" {
		return v
	}

	return filepath.Join(defaults.BaseDir(""), localFolder)
}
