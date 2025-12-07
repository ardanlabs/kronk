// Package tools provides functions for installing and upgrading llama.cpp.
package tools

import (
	"context"
	"fmt"
)

// Logger provides a function for logging messages from DownloadLibraries.
type Logger func(ctx context.Context, msg string, args ...any)

// FmtLogger provides a basic logger that writes to stdout.
var FmtLogger = func(ctx context.Context, msg string, args ...any) {
	fmt.Print(msg)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v[%v]", args[i], args[i+1])
		}
	}
	fmt.Println()
}
