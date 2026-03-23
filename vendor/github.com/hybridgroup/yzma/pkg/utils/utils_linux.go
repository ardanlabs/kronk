package utils

import "golang.org/x/sys/unix"

// BytePtrFromString converts a Go string to a C-style null-terminated byte pointer.
func BytePtrFromString(s string) (*byte, error) {
	return unix.BytePtrFromString(s)
}

// BytePtrToString converts a C-style null-terminated byte pointer to a Go string.
func BytePtrToString(p *byte) string {
	return unix.BytePtrToString(p)
}
