// Package applog re-exports github.com/ardanlabs/kronk/sdk/applog so
// existing imports under sdk/kronk/applog continue to compile. New
// code should depend on sdk/applog directly.
package applog

import (
	"github.com/ardanlabs/kronk/sdk/applog"
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

// NoTraceID is the sentinel value returned by GetTraceID when no
// trace id has been set on the context.
const NoTraceID = applog.NoTraceID

// SetTraceID sets the trace id on the context.
var SetTraceID = applog.SetTraceID

// GetTraceID returns the trace id from the context, or NoTraceID if
// no value has been set.
var GetTraceID = applog.GetTraceID

// DiscardLogger discards logging.
var DiscardLogger = applog.DiscardLogger

// FmtLogger provides a basic logger that writes to stdout.
var FmtLogger = applog.FmtLogger
