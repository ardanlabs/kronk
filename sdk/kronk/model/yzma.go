package model

import (
	"sync"
)

// This file contains workarounds for yzma FFI issues that haven't been
// fixed upstream. These functions wrap or replace yzma functions with
// correct FFI calling conventions.

var (
	yzmaOnce sync.Once
	// yzmaLlamaCppFunction ffi.Fun
)

// InitYzmaWorkarounds loads the mtmd library and preps our fixed FFI functions.
// This is safe to call multiple times; it only initializes once.
func InitYzmaWorkarounds(libPath string) error {
	var initErr error
	yzmaOnce.Do(func() {
		// lib, err := loader.LoadLibrary(libPath, "mtmd")
		// if err != nil {
		// 	initErr = err
		// 	return
		// }

		// Prep the function with correct types:
		// yzmaLlamaCppFunction, err = lib.Prep("llama function", parameters ...)
		// if err != nil {
		// 	initErr = err
		// 	return
		// }
	})
	return initErr
}

// func LlamaCppFunction(<PARAMETERS>) <RETURN> {
// }
