// Package applog defines the canonical Logger function type, log level
// constants, and trace-id context plumbing shared across the Kronk SDK
// packages. It has no dependencies outside the standard library so any
// sub-package can import it without creating a cycle.
package applog

import (
	"context"
	"fmt"
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

// =============================================================================

// NoTraceID is the sentinel value returned by GetTraceID when no trace id
// has been set on the context. The 32-character zero-string matches the
// OpenTelemetry convention for an unset trace id.
const NoTraceID = "00000000000000000000000000000000"

type ctxKey int

const traceIDKey ctxKey = 1

// SetTraceID sets the trace id on the context.
func SetTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

// GetTraceID returns the trace id from the context, or NoTraceID if no
// value has been set.
func GetTraceID(ctx context.Context) string {
	v, ok := ctx.Value(traceIDKey).(string)
	if !ok {
		return NoTraceID
	}

	return v
}

// =============================================================================

// DiscardLogger discards logging.
var DiscardLogger Logger = func(ctx context.Context, msg string, args ...any) {
}

// FmtLogger provides a basic logger that writes to stdout. If a trace id
// has been set on the context via SetTraceID it is included in the output.
var FmtLogger Logger = func(ctx context.Context, msg string, args ...any) {
	if traceID := GetTraceID(ctx); traceID != "" && traceID != NoTraceID {
		fmt.Printf("traceID: %s: %s:", traceID, msg)
	} else {
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
