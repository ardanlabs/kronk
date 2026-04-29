package otel

import (
	"context"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"go.opentelemetry.io/otel/trace"
)

type ctxKey int

const tracerKey ctxKey = 1

// SetTracer sets the tracer in the context.
func SetTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey, tracer)
}

// SetTraceID sets the traceID in the context. It delegates to applog so that
// any code reading the trace id (including applog.FmtLogger) sees the value.
func SetTraceID(ctx context.Context, traceID string) context.Context {
	return applog.SetTraceID(ctx, traceID)
}

// GetTraceID returns the trace id from the context.
func GetTraceID(ctx context.Context) string {
	return applog.GetTraceID(ctx)
}
