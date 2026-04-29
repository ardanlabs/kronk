package kronk

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// Logger provides a function for logging messages from different APIs.
type Logger = applog.Logger

// LogLevel represents the logging level.
type LogLevel = applog.LogLevel

// Set of logging levels supported by llama.cpp.
const (
	LogSilent = applog.LogSilent
	LogNormal = applog.LogNormal
)

// =============================================================================

// DiscardLogger discards logging.
var DiscardLogger = applog.DiscardLogger

// FmtLogger provides a basic logger that writes to stdout.
var FmtLogger = applog.FmtLogger

// SetFmtLoggerTraceID allows you to set a trace id in the content that
// can be part of the output of the FmtLogger.
func SetFmtLoggerTraceID(ctx context.Context, traceID string) context.Context {
	return applog.SetTraceID(ctx, traceID)
}
