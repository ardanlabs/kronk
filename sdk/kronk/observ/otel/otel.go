// Package otel provides otel support.
package otel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

const defaultTraceID = "00000000000000000000000000000000"

// Config defines the information needed to init tracing.
type Config struct {
	ServiceName    string
	Host           string
	ExcludedRoutes map[string]struct{}
	Probability    float64
}

// InitTracing configures open telemetry to be used with the service. It starts
// with a noop tracer provider and if a host is configured, launches a background
// goroutine that checks every 60 seconds for collector availability. When the
// collector becomes reachable, it swaps in a real tracer provider.
func InitTracing(log *logger.Logger, cfg Config) (trace.TracerProvider, func(ctx context.Context), error) {

	// Always start with a noop provider so the service can run without
	// a collector being available.
	otel.SetTracerProvider(noop.NewTracerProvider())

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// If no host is configured, return noop with no background work.
	if cfg.Host == "" {
		log.Info(context.Background(), "OTEL", "tracer", "NOOP", "status", "no host configured")

		return otel.GetTracerProvider(), func(ctx context.Context) {}, nil
	}

	log.Info(context.Background(), "OTEL", "tracer", "NOOP", "status", "starting background collector probe", "host", cfg.Host)

	bgCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var mu sync.Mutex
	var realTP *sdktrace.TracerProvider

	tryConnect := func() bool {
		if !collectorReachable(cfg.Host) {
			return false
		}

		select {
		case <-bgCtx.Done():
			return true
		default:
		}

		tp, err := buildRealProvider(bgCtx, cfg)
		if err != nil {
			log.Info(context.Background(), "OTEL", "status", "collector reachable but init failed", "error", err)
			return false
		}

		mu.Lock()
		realTP = tp
		mu.Unlock()

		otel.SetTracerProvider(tp)

		log.Info(context.Background(), "OTEL", "tracer", cfg.Host, "status", "connected")

		return true
	}

	wg.Go(func() {
		if tryConnect() {
			return
		}

		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-bgCtx.Done():
				return
			case <-ticker.C:
				if tryConnect() {
					return
				}
			}
		}
	})

	teardown := func(ctx context.Context) {
		cancel()
		wg.Wait()

		mu.Lock()
		tp := realTP
		mu.Unlock()

		if tp != nil {
			tp.Shutdown(ctx)
		}
	}

	return otel.GetTracerProvider(), teardown, nil
}

func collectorReachable(host string) bool {
	conn, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func buildRealProvider(ctx context.Context, cfg Config) (*sdktrace.TracerProvider, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exporter, err := otlptrace.New(
		ctx,
		otlptracegrpc.NewClient(
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(cfg.Host),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(newEndpointExcluder(cfg.ExcludedRoutes, cfg.Probability))),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxExportBatchSize(sdktrace.DefaultMaxExportBatchSize),
			sdktrace.WithBatchTimeout(sdktrace.DefaultScheduleDelay*time.Millisecond),
		),
		sdktrace.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(cfg.ServiceName),
			),
		),
	)

	return tp, nil
}

// InjectTracing initializes the request for tracing by writing otel related
// information into the response and saving the tracer and trace id in the
// context for later use.
func InjectTracing(ctx context.Context, tracer trace.Tracer) context.Context {
	ctx = SetTracer(ctx, tracer)

	// If trace ID already exists in context (e.g., propagated from caller), use it.
	if existing := GetTraceID(ctx); existing != defaultTraceID {
		return ctx
	}

	traceID := trace.SpanFromContext(ctx).SpanContext().TraceID().String()
	if traceID == defaultTraceID {
		traceID = uuid.NewString()
	}
	ctx = SetTraceID(ctx, traceID)

	return ctx
}

// AddSpan adds an otel span to the existing trace.
func AddSpan(ctx context.Context, spanName string, keyValues ...attribute.KeyValue) (context.Context, trace.Span) {
	tracer, ok := ctx.Value(tracerKey).(trace.Tracer)
	if !ok || tracer == nil {
		return ctx, trace.SpanFromContext(ctx)
	}

	ctx, span := tracer.Start(ctx, spanName)

	span.SetAttributes(keyValues...)

	return ctx, span
}

// AddTraceToRequest adds the current trace id to the request so it
// can be delivered to the service being called.
func AddTraceToRequest(ctx context.Context, r *http.Request) {
	hc := propagation.HeaderCarrier(r.Header)
	otel.GetTextMapPropagator().Inject(ctx, hc)
}
