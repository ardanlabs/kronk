//go:build ((freebsd || linux || windows) && arm64) || (linux && riscv64)

package ffi

const TrampolineSize = 24
