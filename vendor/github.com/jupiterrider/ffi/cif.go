//go:build ((freebsd || linux || windows || darwin) && amd64) || ((freebsd || linux) && arm64)

package ffi

// Cif stands for "Call InterFace". It describes the signature of a function.
//
// Use [PrepCif] to initialize it.
type Cif struct {
	Abi      uint32
	NArgs    uint32
	ArgTypes **Type
	RType    *Type
	Bytes    uint32
	Flags    uint32
}
