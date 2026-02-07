package mid

import (
	"context"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/observ/otel"
	"go.opentelemetry.io/otel/trace"
)

// Tracer injects the tracer from the current span context into the SDK's
// otel context key so that otel.AddSpan can create child spans.
func Tracer() web.MidFunc {
	m := func(next web.HandlerFunc) web.HandlerFunc {
		h := func(ctx context.Context, r *http.Request) web.Encoder {
			span := trace.SpanFromContext(ctx)
			tracer := span.TracerProvider().Tracer("")

			ctx = otel.SetTracer(ctx, tracer)

			return next(ctx, r)
		}

		return h
	}

	return m
}
