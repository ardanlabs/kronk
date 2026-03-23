//go:build ((freebsd || linux || windows || darwin) && (amd64 || arm64)) || (linux && riscv64)

package ffi

import (
	"runtime"
)

// filename is the name or path to the libffi shared library.
var filename string

func init() {
	if len(filename) == 0 {
		switch runtime.GOOS {
		case "freebsd", "linux":
			filename = "libffi.so.8"
		case "windows":
			filename = "libffi-8.dll"
		case "darwin":
			filename = "libffi.8.dylib"
		}
	}

	libffi, err := Load(filename)
	if err != nil {
		panic(err)
	}

	prepCif, err = libffi.Get("ffi_prep_cif")
	if err != nil {
		panic(err)
	}

	prepCifVar, err = libffi.Get("ffi_prep_cif_var")
	if err != nil {
		panic(err)
	}

	call, err = libffi.Get("ffi_call")
	if err != nil {
		panic(err)
	}

	closureAlloc, err = libffi.Get("ffi_closure_alloc")
	if err != nil {
		panic(err)
	}

	closureFree, err = libffi.Get("ffi_closure_free")
	if err != nil {
		panic(err)
	}

	prepClosureLoc, err = libffi.Get("ffi_prep_closure_loc")
	if err != nil {
		panic(err)
	}

	getStructOffsets, err = libffi.Get("ffi_get_struct_offsets")
	if err != nil {
		panic(err)
	}

	// Because ffi_get_version and ffi_get_version_number just exist since libffi 3.5.0, we don't panic here.
	getVersion, _ = libffi.Get("ffi_get_version")

	getVersionNumber, _ = libffi.Get("ffi_get_version_number")
}
