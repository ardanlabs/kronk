//go:build ((freebsd || linux || darwin) && arm64) || (windows && amd64) || (linux && riscv64)

package ffi

const (
	DefaultAbi Abi = 1
)
