package web

import (
	"context"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ctxKey int

const (
	tracerKey ctxKey = iota + 1
	writerKey
	traceIDKey
)

const defaultTraceID = "00000000000000000000000000000000"

func setTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey, id)
}

// GetTraceID returns the trace id from the context.
func GetTraceID(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDKey).(string); ok {
		return v
	}

	return defaultTraceID
}

func setTracer(ctx context.Context, tracer trace.Tracer) context.Context {
	return context.WithValue(ctx, tracerKey, tracer)
}

func addSpan(ctx context.Context, spanName string, keyValues ...attribute.KeyValue) (context.Context, trace.Span) {
	v, ok := ctx.Value(tracerKey).(trace.Tracer)
	if !ok || v == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := v.Start(ctx, spanName)
	span.SetAttributes(keyValues...)

	return ctx, span
}

func setWriter(ctx context.Context, w http.ResponseWriter) context.Context {
	return context.WithValue(ctx, writerKey, w)
}

// GetWriter returns the underlying writer for the request.
func GetWriter(ctx context.Context) http.ResponseWriter {
	v, ok := ctx.Value(writerKey).(http.ResponseWriter)
	if !ok {
		return nil
	}

	return v
}
