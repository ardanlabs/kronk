package download

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
)

// AlreadyInstalled checks if whisper.cpp is already installed at the given
// libPath. It does this by checking for the presence of the library file
// corresponding to the current OS. If the library file exists, it returns
// true, indicating that whisper.cpp is already installed. If the library
// file does not exist, it returns false.
func AlreadyInstalled(libPath string) bool {
	if _, err := os.Stat(filepath.Join(libPath, LibraryName(runtime.GOOS))); !errors.Is(err, os.ErrNotExist) {
		return true
	}
	return false
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
