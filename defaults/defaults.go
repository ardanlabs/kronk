// Package defaults provides default values for the cli tooling.
package defaults

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/ardanlabs/kronk"
	"github.com/hybridgroup/yzma/pkg/download"
)

var (
	basePath  = ".kronk"
	libsPath  = "libraries"
	modelPath = "models"
)

// BaseDir is the default base folder location for kronk files.
func BaseDir(override string) string {
	if override != "" {
		return override
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("./%s", basePath)
	}

	return filepath.Join(homeDir, basePath)
}

// LibsDir returns the default location for the libraries folder. It will check
// the KRONK_LIB_PATH env var first and then default to the home directory if
// one can be identified. Last resort it will choose the current directory.
func LibsDir(override string) string {
	if override != "" {
		return override
	}

	if v := os.Getenv("KRONK_LIB_PATH"); v != "" {
		return v
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("./%s/%s", basePath, libsPath)
	}

	return filepath.Join(homeDir, basePath, libsPath)
}

// ModelsDir returns the default location for the models folder. It will check
// the KRONK_MODELS env var first and then default to the home directory if one
// can be identified. Last resort it will choose the current directory.
func ModelsDir(override string) string {
	if override != "" {
		return override
	}

	if v := os.Getenv("KRONK_MODELS"); v != "" {
		return v
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Sprintf("./%s/%s", basePath, modelPath)
	}

	return filepath.Join(homeDir, basePath, modelPath)
}

// Arch will check the KRONK_ARCH var first and check it's value against the
// proper set of architectures. If that variable is not set, then runtime.GOARCH
// is used.
func Arch(override string) (download.Arch, error) {
	if override != "" {
		return download.ParseArch(override)
	}

	if v := os.Getenv("KRONK_ARCH"); v != "" {
		return download.ParseArch(v)
	}

	return download.ParseArch(runtime.GOARCH)
}

// OS will check the KRONK_OS var first and check it's value against the proper
// set of operating systems. If that variable is not set, then runtime.GOOS
//
//	is used.
func OS(override string) (download.OS, error) {
	if override != "" {
		return download.ParseOS(override)
	}

	if v := os.Getenv("KRONK_OS"); v != "" {
		return download.ParseOS(v)
	}

	return download.ParseOS(runtime.GOOS)
}

// Processor will check the KRONK_PROCESSOR env var first and check it's value
// against the proper set of processor values (cpu, cuda, metal, vulkan). If
// that variable is not set, then cpu is used as the default.
func Processor(override string) (download.Processor, error) {
	if override != "" {
		return download.ParseProcessor(override)
	}

	if v := os.Getenv("KRONK_PROCESSOR"); v != "" {
		return download.ParseProcessor(v)
	}

	return download.CPU, nil
}

func LlamaLog(override int) (kronk.LogLevel, error) {
	if override < 1 || override > 2 {
		return 0, fmt.Errorf("invalid log level %d, want slient(1) or normal(2)", override)
	}

	return kronk.LogLevel(override), nil
}
