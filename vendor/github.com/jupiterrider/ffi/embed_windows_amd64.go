//go:build !ffi_no_embed

package ffi

import _ "embed"

const libname = "libffi-8.dll"

//go:embed assets/libffi/windows_amd64/libffi-8.dll
var embeddedLib []byte
