//go:build !ffi_no_embed

package ffi

import _ "embed"

const libname = "libffi.8.dylib"

//go:embed assets/libffi/darwin_arm64/libffi.8.dylib
var embeddedLib []byte
