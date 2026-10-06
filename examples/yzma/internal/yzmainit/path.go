// Package yzmainit prepares Kronk's llama.cpp installation for direct use by
// the raw Yzma examples.
package yzmainit

import "github.com/ardanlabs/kronk/sdk/tools/libs"

// LibraryPath returns the selected llama.cpp installation after configuring
// platform-specific shared-library discovery for the current process.
func LibraryPath() (string, error) {
	libPath := libs.Path("")
	if err := prepareLibraryPath(libPath); err != nil {
		return "", err
	}

	return libPath, nil
}
