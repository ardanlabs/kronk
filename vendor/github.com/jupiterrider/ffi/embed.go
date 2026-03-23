//go:build !ffi_no_embed && (darwin || (windows && amd64))

package ffi

import (
	_ "embed"
	"os"
	"path/filepath"
)

const libffiVersion = "3.5.1"

//go:embed assets/libffi/LICENSE
var embeddedLicense []byte

func init() {
	if os.Getenv("FFI_NO_EMBED") == "1" {
		return
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return
	}

	destDir := filepath.Join(userCacheDir, "github.com/jupiterrider/ffi/libffi", libffiVersion)
	destLib := filepath.Join(destDir, libname)
	destLicense := filepath.Join(destDir, "LICENSE")

	if os.MkdirAll(destDir, 0755) == nil {
		if fileInfo, err := os.Stat(destLib); err != nil {
			if os.IsNotExist(err) {
				if os.WriteFile(destLib, embeddedLib, 0755) == nil {
					filename = destLib
				}
			}
		} else {
			if fileInfo != nil && !fileInfo.IsDir() {
				filename = destLib
			}
		}

		if _, err := os.Stat(destLicense); err != nil {
			if os.IsNotExist(err) {
				os.WriteFile(destLicense, embeddedLicense, 0644)
			}
		}
	}
}
