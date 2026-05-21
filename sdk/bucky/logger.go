package bucky

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// Logger provides a function for logging messages from different APIs.
type Logger = applog.Logger

// LogLevel represents the logging level.
type LogLevel = applog.LogLevel

// Set of logging levels supported by whisper.cpp.
const (
	LogSilent = applog.LogSilent
	LogNormal = applog.LogNormal
)

// =============================================================================

// DiscardLogger discards logging.
var DiscardLogger = applog.DiscardLogger

// FmtLogger provides a basic logger that writes to stdout.
var FmtLogger = applog.FmtLogger

// SetFmtLoggerTraceID allows you to set a trace id on the context
// that can be included in FmtLogger output.
func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context {
	return applog.SetTraceID(ctx, traceID)
}
