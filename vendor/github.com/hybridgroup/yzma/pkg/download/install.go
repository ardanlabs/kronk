package download

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

var (
	llamaCppVersionDocURL = "https://api.github.com/repos/hybridgroup/llama-cpp-builder/releases/latest"
	versionFile           = "version.json"
)

type tag struct {
	TagName string `json:"tag_name"`
}

// AlreadyInstalled checks if llama.cpp is already installed at the given libPath. It does this
// by checking for the presence of the library file corresponding to the current OS. If the
// library file exists, it returns true, indicating that llama.cpp is already installed. If the
// library file does not exist, it returns false, indicating that llama.cpp is not installed.
func AlreadyInstalled(libPath string) bool {
	if _, err := os.Stat(filepath.Join(libPath, LibraryName(runtime.GOOS))); !os.IsNotExist(err) {
		return true
	}
	return false
}

// InstallLibraries has been deprecated. Use the `GetXXX` functions directly.
func InstallLibraries(libPath string, processor Processor, allowUpgrade bool) error {
	return fmt.Errorf("InstallLibraries is deprecated. Use the GetXXX functions directly")
}

var execCommand = exec.Command

// HasCUDA checks if CUDA is available and returns (available, cudaVersion).
func HasCUDA() (bool, string) {
	if runtime.GOOS == "darwin" {
		return false, ""
	}

	cmd := execCommand("nvidia-smi")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return false, ""
	}
	re := regexp.MustCompile(`CUDA Version:\s*([0-9.]+)`)
	matches := re.FindStringSubmatch(out.String())
	if len(matches) >= 2 {
		return true, matches[1]
	}
	return true, ""
}

// HasROCm checks if ROCm is available and returns (available, rocmVersion).
func HasROCm() (bool, string) {
	if runtime.GOOS != "linux" {
		return false, ""
	}

	cmd := execCommand("rocminfo")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil {
		return false, ""
	}
	re := regexp.MustCompile(`Runtime Version:\s*([0-9.]+)`)
	matches := re.FindStringSubmatch(out.String())
	if len(matches) >= 2 {
		return true, matches[1]
	}
	return true, ""
}
