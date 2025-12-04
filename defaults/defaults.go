// Package defaults provides default values for the cli tooling.
package defaults

import (
	"os"
	"path/filepath"

	"github.com/hybridgroup/yzma/pkg/download"
)

// LibsDir returns the default location for the libraries folder. It will check
// the KRONK_LIBRARIES env var first and then default to the home directory if
// one can be identified. Last resort it will choose the current directory.
func LibsDir() string {
	if v := os.Getenv("KRONK_LIBRARIES"); v != "" {
		return v
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./kronk/libraries"
	}

	return filepath.Join(homeDir, "kronk/libraries")
}

// ModelsDir returns the default location for the models folder. It will check
// the KRONK_MODELS env var first and then default to the home directory if one
// can be identified. Last resort it will choose the current directory.
func ModelsDir() string {
	if v := os.Getenv("KRONK_MODELS"); v != "" {
		return v
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "./kronk/models"
	}

	return filepath.Join(homeDir, "kronk/models")
}

// Processor will check the KRONK_PROCESSOR env var first and check it's value
// against the proper set of processor values (cpu, cuda, metal, vulkan). If
// that variable is not set, then cpu is used as the default.
func Processor() (download.Processor, error) {
	if v := os.Getenv("KRONK_PROCESSOR"); v != "" {
		return download.ParseProcessor(v)
	}

	return download.CPU, nil
}
