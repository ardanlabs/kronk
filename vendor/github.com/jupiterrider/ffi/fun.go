//go:build ((freebsd || linux || windows || darwin) && (amd64 || arm64)) || (linux && riscv64)

package ffi

import (
	"reflect"
	"unsafe"
)

// Fun is used to group a function's address and the related call interface together.
//
// You can use [Lib.Prep] or [Lib.PrepVar] to create one.
type Fun struct {
	Addr uintptr
	Cif  *Cif
}

// Call calls the function's address according to the description given in [Cif].
//   - ret is a pointer to a variable that will hold the result of the function call. Provide nil if the function has no return value.
//     You cannot use integer types smaller than 8 bytes here (float32 and structs are not affected). Use [Arg] instead and typecast afterwards.
//   - args are pointers to the argument values. Leave empty if the function takes none.
//     It panics if the number of arguments doesn't match the prepared Cif.
//
// Example:
//
//	// C function:
//	// int ilogb(double x);
//
//	var result ffi.Arg
//	x := 1.0
//	ilogb.Call(&result, &x)
//	fmt.Printf("%d\n", int32(result))
func (f Fun) Call(ret any, args ...any) {
	var rV unsafe.Pointer
	if ret != nil {
		rV = reflect.ValueOf(ret).UnsafePointer()
	}

	nArgs := len(args)

	if nArgs > int(f.Cif.NArgs) {
		panic("ffi: calling with too many arguments")
	} else if nArgs < int(f.Cif.NArgs) {
		panic("ffi: calling with too few arguments")
	}

	aV := make([]unsafe.Pointer, nArgs)

	for i := 0; i < nArgs; i++ {
		aV[i] = reflect.ValueOf(args[i]).UnsafePointer()
	}

	Call(f.Cif, f.Addr, rV, aV...)
}
