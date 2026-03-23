//go:build ((freebsd || linux || darwin) && (amd64 || arm64)) || (linux && riscv64)

package ffi

import (
	"github.com/ebitengine/purego"
)

// Load loads a shared library at runtime.
//
// The name can be an absolute path, relative path or just the filename.
// If just the filename is passed, it will use the OS specific search paths.
//
// Example:
//
//	var filename string
//
//	switch runtime.GOOS {
//	case "freebsd", "linux":
//		filename = "libraylib.so"
//	case "windows":
//		filename = "raylib.dll"
//	case "darwin":
//		filename = "libraylib.dylib"
//	}
//
//	raylib, err := ffi.Load(filename)
func Load(name string) (l Lib, err error) {
	l.Addr, err = purego.Dlopen(name, purego.RTLD_LAZY)
	return
}

// Get retrieves the address of an exported function or variable.
//
// Example:
//
//	// C code:
//	// int magic_number = 42;
//
//	magicNumber, err := lib.Get("magic_number")
//	if err != nil {
//		panic(err)
//	}
//
//	// prints 42
//	fmt.Println(*(*int32)(unsafe.Pointer(magicNumber)))
//
//	// if go vet yells "possible misuse of unsafe.Pointer",
//	// you can do the following workaround:
//	fmt.Println(**(**int32)(unsafe.Pointer(&magicNumber)))
func (l Lib) Get(name string) (addr uintptr, err error) {
	return purego.Dlsym(l.Addr, name)
}

// Close deletes a reference to the library. If the reference count is zero,
// the library gets unloaded.
func (l Lib) Close() error {
	if l.Addr == 0 {
		return nil
	}

	return purego.Dlclose(l.Addr)
}
