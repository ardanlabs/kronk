//go:build ((freebsd || linux || darwin) && amd64) || (windows && arm64)

package ffi

const (
	DefaultAbi Abi = 2
)
