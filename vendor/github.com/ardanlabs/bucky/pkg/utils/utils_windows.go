package utils

import "golang.org/x/sys/windows"

// BytePtrFromString converts a Go string to a C-style null-terminated byte pointer.
func BytePtrFromString(s string) (*byte, error) {
	return windows.BytePtrFromString(s)
}

// BytePtrToString converts a C-style null-terminated byte pointer to a Go string.
func BytePtrToString(p *byte) string {
	return windows.BytePtrToString(p)
}
