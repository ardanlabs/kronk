//go:build ((freebsd || linux || windows || darwin) && (amd64 || arm64)) || (linux && riscv64)

package ffi

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

var prepCif, prepCifVar, call, closureAlloc, closureFree, prepClosureLoc, getStructOffsets, getVersion, getVersionNumber uintptr

type Abi uint32

// Arg can be used as a return value for functions, which return integers smaller than 8 bytes.
//
// See [Call].
type Arg uint64

// Bool converts [Arg] into a Boolean.
func (a Arg) Bool() bool {
	return byte(a) != 0
}

type Status uint32

const (
	OK Status = iota
	BadTypedef
	BadAbi
	BadArgType
)

func (s Status) String() string {
	status := map[Status]string{OK: "OK", BadTypedef: "bad type definition", BadAbi: "bad ABI", BadArgType: "bad argument type"}
	return status[s]
}

// These constants are used for the Type field of [Type].
const (
	Void = iota
	Int
	Float
	Double
	Longdouble
	Uint8
	Sint8
	Uint16
	Sint16
	Uint32
	Sint32
	Uint64
	Sint64
	Struct
	Pointer
	Complex
)

// Type is used to describe the structure of a data type.
//
// Example:
//
//	// C struct:
//	// typedef struct Point {
//	//     int x;
//	//     int y;
//	// } Point;
//
//	typePoint := ffi.Type{Type: ffi.Struct, Elements: &[]*ffi.Type{&ffi.TypeSint32, &ffi.TypeSint32, nil}[0]}
//
// Primitive data types are already defined (e.g. [TypeDouble] for float64).
type Type struct {
	Size      uint64 // Initialize to 0 (automatically set by libffi as needed).
	Alignment uint16 // Initialize to 0 (automatically set by libffi as needed).
	Type      uint16 // Use ffi.Struct for struct types.
	Elements  **Type // Pointer to the first element of a nil-terminated slice.
}

// NewType can by used to create a new struct [Type].
//
// Example:
//
//	ffi.NewType(&ffi.TypeFloat, &ffi.TypeFloat)
//	// is equivalent to
//	ffi.Type{Type: ffi.Struct, Elements: &[]*ffi.Type{&ffi.TypeFloat, &ffi.TypeFloat, nil}[0]}
func NewType(elements ...*Type) Type {
	elements = append(elements, nil)
	return Type{Type: Struct, Elements: &elements[0]}
}

// Closure can be used to create callbacks (function pointers) at runtime.
//
// Use [ClosureAlloc] for allocation and [PrepClosureLoc] for preparation.
type Closure struct {
	Tramp    [TrampolineSize]byte
	Cif      *Cif
	Fun      unsafe.Pointer
	UserData unsafe.Pointer
}

// Callback has the following parameters:
//   - cif is the same object that was passed to [PrepClosureLoc].
//   - ret is a pointer to the memory used for the function's return value.
//     If the function is declared as returning void, then this value is garbage and should not be used.
//   - args is a C array of pointers to the arguments. You can use [unsafe.Slice] to convert it.
//   - userData is the same data that was passed to [PrepClosureLoc].
//
// The returned uintptr can be ignored. It's just there for compatibility reasons.
//
// Example:
//
//	// We assume that the closure has the following signature:
//	// int64_t Add(int64_t a, int64_t b);
//
//	cb := ffi.NewCallback(func(cif *ffi.Cif, ret unsafe.Pointer, args *unsafe.Pointer, userData unsafe.Pointer) uintptr {
//		arguments := unsafe.Slice(args, cif.NArgs)
//		*(*int64)(ret) = *(*int64)(arguments[0]) + *(*int64)(arguments[1])
//		return 0
//	})
type Callback func(cif *Cif, ret unsafe.Pointer, args *unsafe.Pointer, userData unsafe.Pointer) uintptr

// NewCallback converts the Go function fn into a C function pointer.
// The returned value can be used as parameter in [PrepClosureLoc].
func NewCallback(fn Callback) uintptr {
	return purego.NewCallback(fn)
}

// PrepCif initializes cif.
//   - abi is the ABI to use. Normally [DefaultAbi] is what you want.
//   - nArgs is the number of arguments. Use 0 if the function has none.
//   - rType is the return type. Use [TypeVoid] if the function has none.
//   - aTypes are the arguments. Leave empty if the function has none.
//
// The returned status code will be [OK], if everything worked properly.
//
// Example:
//
//	// C function:
//	// double cos(double x);
//
//	var cif ffi.Cif
//	status := ffi.PrepCif(&cif, ffi.DefaultAbi, 1, &ffi.TypeDouble, &ffi.TypeDouble)
//	if status != ffi.OK {
//		panic(status)
//	}
func PrepCif(cif *Cif, abi Abi, nArgs uint32, rType *Type, aTypes ...*Type) Status {
	if len(aTypes) > 0 {
		ret, _, _ := purego.SyscallN(prepCif, uintptr(unsafe.Pointer(cif)), uintptr(abi), uintptr(nArgs), uintptr(unsafe.Pointer(rType)), uintptr(unsafe.Pointer(&aTypes[0])))
		return Status(ret)
	}
	ret, _, _ := purego.SyscallN(prepCif, uintptr(unsafe.Pointer(cif)), uintptr(abi), uintptr(nArgs), uintptr(unsafe.Pointer(rType)))
	return Status(ret)
}

// PrepCifVar initializes cif for a call to a variadic function.
//
// In general its operation is the same as for [PrepCif] except that:
//   - nFixedArgs is the number of fixed arguments, prior to any variadic arguments. It must be greater than zero.
//   - nTotalArgs is the total number of arguments, including variadic and fixed arguments. aTypes must have this many elements.
//
// This function will return [BadArgType] if any of the variable argument types is [TypeFloat].
// Same goes for integer types smaller than 4 bytes. See [issue 608].
//
// Note that, different cif's must be prepped for calls to the same function when different numbers of arguments are passed.
//
// Also note that a call to this function with nFixedArgs = nTotalArgs is NOT equivalent to a call to [PrepCif].
//
// Example:
//
//	// C function:
//	// int printf(const char *restrict format, ...);
//
//	var cif ffi.Cif
//	status := ffi.PrepCifVar(&cif, ffi.DefaultAbi, 1, 2, &ffi.TypeSint32, &ffi.TypePointer, &ffi.TypeDouble)
//	if status != ffi.OK {
//		panic(status)
//	}
//
//	text, _ := unix.BytePtrFromString("Pi is %f\n")
//	pi := math.Pi
//	var nCharsPrinted int32
//	ffi.Call(&cif, printf, unsafe.Pointer(&nCharsPrinted), unsafe.Pointer(&text), unsafe.Pointer(&pi))
//
// [issue 608]: https://github.com/libffi/libffi/issues/608
func PrepCifVar(cif *Cif, abi Abi, nFixedArgs, nTotalArgs uint32, rType *Type, aTypes ...*Type) Status {
	const intSize = 4

	// This check has been rebuild according to the original: https://github.com/libffi/libffi/blob/v3.4.6/src/prep_cif.c#L244
	//
	// Without rebuild, the type check wouldn't work for float,
	// because libffi compares the pointer to ffi_type_float instead of value equality.
	for i := nFixedArgs; i < nTotalArgs; i++ {
		argType := *aTypes[i]
		if argType == TypeFloat || ((argType.Type != Struct && argType.Type != Complex) && argType.Size < intSize) {
			return BadArgType
		}
	}

	if len(aTypes) > 0 {
		ret, _, _ := purego.SyscallN(prepCifVar, uintptr(unsafe.Pointer(cif)), uintptr(abi), uintptr(nFixedArgs), uintptr(nTotalArgs), uintptr(unsafe.Pointer(rType)), uintptr(unsafe.Pointer(&aTypes[0])))
		return Status(ret)
	}
	ret, _, _ := purego.SyscallN(prepCifVar, uintptr(unsafe.Pointer(cif)), uintptr(abi), uintptr(nFixedArgs), uintptr(nTotalArgs), uintptr(unsafe.Pointer(rType)))
	return Status(ret)
}

// Call calls the function fn according to the description given in cif. cif must have already been prepared using [PrepCif].
//   - fn is the address of the desired function. Use [purego.Dlsym] to get one.
//   - rValue is a pointer to a variable that will hold the result of the function call. Provide nil if the function has no return value.
//     You cannot use integer types smaller than 8 bytes here (float32 and structs are not affected). Use [Arg] instead and typecast afterwards.
//   - aValues are pointers to the argument values. Leave empty if the function takes none.
//
// Example:
//
//	// C function:
//	// int ilogb(double x);
//
//	var result ffi.Arg
//	x := 1.0
//	ffi.Call(&cif, ilogb, unsafe.Pointer(&result), unsafe.Pointer(&x))
//	fmt.Printf("%d\n", int32(result))
func Call(cif *Cif, fn uintptr, rValue unsafe.Pointer, aValues ...unsafe.Pointer) {
	if len(aValues) > 0 {
		purego.SyscallN(call, uintptr(unsafe.Pointer(cif)), fn, uintptr(rValue), uintptr(unsafe.Pointer(&aValues[0])))
		return
	}
	purego.SyscallN(call, uintptr(unsafe.Pointer(cif)), fn, uintptr(rValue))
}

// ClosureAlloc allocates a new [Closure].
//   - size should be big enough to hold a [Closure] object.
//   - code is the corresponding executable address (function pointer).
//
// The Closure is not managed by Go's garbage collector. It can be deallocated by using [ClosureFree].
//
// Example:
//
//	var code unsafe.Pointer
//	closure := ffi.ClosureAlloc(unsafe.Sizeof(ffi.Closure{}), &code)
//	defer ffi.ClosureFree(closure)
func ClosureAlloc(size uintptr, code *unsafe.Pointer) *Closure {
	ret, _, _ := purego.SyscallN(closureAlloc, size, uintptr(unsafe.Pointer(code)))
	return *(**Closure)(unsafe.Pointer(&ret))
}

// ClosureFree is used to free memory allocated by [ClosureAlloc].
func ClosureFree(writable *Closure) {
	purego.SyscallN(closureFree, uintptr(unsafe.Pointer(writable)))
}

// PrepClosureLoc creates a C function (so-called closure) at runtime.
//   - closure is the object return by [ClosureAlloc].
//   - cif describes the signature of the function to be created. Use [PrepCif] for initialization.
//   - fun is a pointer to a C function which will be called when the closure is invoked. You can use [NewCallback] to create one.
//   - userData is arbitrary and optional (can be nil) data passed to your closure function fun.
//   - codeLoc is the executable address allocated by [ClosureAlloc].
func PrepClosureLoc(closure *Closure, cif *Cif, fun uintptr, userData, codeLoc unsafe.Pointer) Status {
	ret, _, _ := purego.SyscallN(prepClosureLoc, uintptr(unsafe.Pointer(closure)), uintptr(unsafe.Pointer(cif)), fun, uintptr(userData), uintptr(codeLoc))
	return Status(ret)
}

// GetStructOffsets computes the offset of each element of the given structure type and sets the size and alignment.
//
// offsets is an optional out parameter (pointer to the first element of an array).
// The array's length should be equal to the number of struct fields.
func GetStructOffsets(abi Abi, structType *Type, offsets *uint64) Status {
	ret, _, _ := purego.SyscallN(getStructOffsets, uintptr(abi), uintptr(unsafe.Pointer(structType)), uintptr(unsafe.Pointer(offsets)))
	return Status(ret)
}

// GetVersion returns the version of the loaded libffi library (not the Go binding), or "" if not available.
//
// This function requires libffi 3.5.0 or newer.
func GetVersion() string {
	if getVersion == 0 {
		return ""
	}
	ret, _, _ := purego.SyscallN(getVersion)
	if ret == 0 {
		return ""
	}

	// The following logic converts the null-terminated C-style string to a Go string
	// by determining the position of the trailing null byte:
	p := *(**byte)(unsafe.Pointer(&ret))
	i := 0
	for ptr := unsafe.Pointer(p); *(*byte)(unsafe.Add(ptr, i)) != 0; i++ {
	}
	return string(unsafe.Slice(p, i))
}

// GetVersionNumber returns the version number (major*10000 + minor*100 + patch) of the loaded libffi library (not the Go binding),
// or 0 if not available.
//
// This function requires libffi 3.5.0 or newer.
//
// Example:
//
//	version := ffi.GetVersionNumber()
//	major := version / 10000
//	minor := version / 100 % 100
//	patch := version % 100
//	fmt.Printf("%d.%d.%d\n", major, minor, patch)
func GetVersionNumber() uint64 {
	if getVersionNumber == 0 {
		return 0
	}
	ret, _, _ := purego.SyscallN(getVersionNumber)
	return uint64(ret)
}
