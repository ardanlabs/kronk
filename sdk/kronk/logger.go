package kronk

import (
	"context"
	"fmt"
	"sync"
)

// Logger provides a function for logging messages from different APIs.
type Logger func(ctx context.Context, msg string, args ...any)

// LogLevel represents the logging level.
type LogLevel int

// Int returns the integer value.
func (ll LogLevel) Int() int {
	return int(ll)
}

// Set of logging levels supported by llama.cpp.
const (
	LogSilent LogLevel = iota + 1
	LogNormal
)

var (
	libraryLocation string
	initOnce        sync.Once
	initErr         error
)

// =============================================================================

type traceIDKey int

// DiscardLogger discards logging.
var DiscardLogger = func(ctx context.Context, msg string, args ...any) {
}

// FmtLogger provides a basic logger that writes to stdout.
var FmtLogger = func(ctx context.Context, msg string, args ...any) {
	traceID, ok := ctx.Value(traceIDKey(1)).(string)
	switch ok {
	case true:
		fmt.Printf("traceID: %s: %s:", traceID, msg)
	default:
		fmt.Printf("%s:", msg)
	}

	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			fmt.Printf(" %v[%v]", args[i], args[i+1])
		}
	}

	if len(msg) > 0 && msg[0] != '\r' {
		fmt.Println()
	}
}

// SetFmtLoggerTraceID allows you to set a trace id in the content that
// can be part of the output of the FmtLogger.
func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey(1), traceID)
}
