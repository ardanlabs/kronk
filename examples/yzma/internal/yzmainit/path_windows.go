//go:build windows

package yzmainit

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// prepareLibraryPath preloads llama.dll with its directory at the front of
// the dependency search path. Yzma's later loads reuse the resident bundle.
func prepareLibraryPath(libPath string) error {
	filename, err := filepath.Abs(filepath.Join(libPath, "llama.dll"))
	if err != nil {
		return err
	}

	_, err = windows.LoadLibraryEx(filename, 0, windows.LOAD_WITH_ALTERED_SEARCH_PATH)
	return err
}
