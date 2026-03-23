//go:build ((freebsd || linux || windows || darwin) && (amd64 || arm64)) || (linux && riscv64)

package ffi

import "fmt"

// Lib holds the address to a shared library.
//
// Use [Load] to initialize one.
type Lib struct {
	Addr uintptr
}

// Prep is used to get and describe a library's function.
//   - name is the name of the function.
//   - ret is the return type. Use [TypeVoid] if the function has none.
//   - args are the arguments. Leave empty if the function has none.
//
// For variadic functions use [Lib.PrepVar] instead.
//
// Example:
//
//	// C function:
//	// double cos(double x);
//
//	cos, err := libm.Prep("cos", &ffi.TypeDouble, &ffi.TypeDouble)
//	if err != nil {
//		panic(err)
//	}
func (l Lib) Prep(name string, ret *Type, args ...*Type) (f Fun, err error) {
	if f.Addr, err = l.Get(name); err != nil {
		return
	}

	f.Cif = new(Cif)
	if status := PrepCif(f.Cif, DefaultAbi, uint32(len(args)), ret, args...); status != OK {
		return f, fmt.Errorf("%s: error preparing function: %s", name, status)
	}

	return
}

// PrepVar is used to get and describe a variadic function.
//
// In general its operation is the same as for [Lib.Prep] except that:
//   - nFixedArgs is the number of fixed arguments, prior to any variadic arguments. It must be greater than zero.
//
// This function will return an error if any of the variable argument types is [TypeFloat].
// Same goes for integer types smaller than 4 bytes. See [issue 608].
//
// Note that, different cif's must be prepped for calls to the same function when different numbers of arguments are passed.
//
// Also note that a call to this function with nFixedArgs = len(args) is NOT equivalent to a call to [Lib.Prep].
//
// Example:
//
//	// C function:
//	// int printf(const char *restrict format, ...);
//
//	printf, err := libc.PrepVar("printf", 1, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeDouble)
//	if err != nil {
//		panic(err)
//	}
//
//	text, _ := unix.BytePtrFromString("Pi is %f\n")
//	pi := math.Pi
//	var nCharsPrinted int32
//	printf.Call(&nCharsPrinted, &text, &pi)
//
// [issue 608]: https://github.com/libffi/libffi/issues/608
func (l Lib) PrepVar(name string, nFixedArgs int, ret *Type, args ...*Type) (f Fun, err error) {
	if f.Addr, err = l.Get(name); err != nil {
		return
	}

	f.Cif = new(Cif)
	if status := PrepCifVar(f.Cif, DefaultAbi, uint32(nFixedArgs), uint32(len(args)), ret, args...); status != OK {
		return f, fmt.Errorf("%s: error preparing function: %s", name, status)
	}

	return
}
