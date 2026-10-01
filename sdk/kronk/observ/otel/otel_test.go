package otel

import (
	"context"
	"os"
	"os/exec"
	"testing"

	gootel "go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInitTracingPreservesGlobalDelegation(t *testing.T) {
	if os.Getenv("KRONK_OTEL_DELEGATION_HELPER") == "1" {
		testInitTracingPreservesGlobalDelegation(t)
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestInitTracingPreservesGlobalDelegation$")
	cmd.Env = append(os.Environ(), "KRONK_OTEL_DELEGATION_HELPER=1")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess: %v\n%s", err, output)
	}
}

func testInitTracingPreservesGlobalDelegation(t *testing.T) {
	provider, teardown, err := InitTracing(func(context.Context, string, ...any) {}, Config{})
	if err != nil {
		t.Fatalf("InitTracing: %v", err)
	}
	defer teardown(context.Background())

	tracer := provider.Tracer("saved")
	_, before := tracer.Start(context.Background(), "before-activation")
	if before.IsRecording() {
		t.Fatal("span before activation: got recording, want non-recording")
	}
	before.End()

	recorder := tracetest.NewSpanRecorder()
	realProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	t.Cleanup(func() {
		if err := realProvider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	gootel.SetTracerProvider(realProvider)

	_, after := tracer.Start(context.Background(), "after-activation")
	if !after.IsRecording() {
		t.Fatal("span after activation: got non-recording, want recording")
	}
	after.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans: got %d, want 1", len(ended))
	}
	if got := ended[0].Name(); got != "after-activation" {
		t.Errorf("span name: got %q, want %q", got, "after-activation")
	}
}

func TestAddSpanWithoutTracerDoesNotEndParent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSpanProcessor(recorder),
	)
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	})

	ctx, parent := provider.Tracer("test").Start(context.Background(), "parent")
	_, child := AddSpan(ctx, "child")
	child.End()

	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("ended spans after child.End: got %d, want 0", got)
	}

	parent.End()
	ended := recorder.Ended()
	if got := len(ended); got != 1 {
		t.Fatalf("ended spans after parent.End: got %d, want 1", got)
	}
	if got := ended[0].Name(); got != "parent" {
		t.Errorf("ended span: got %q, want %q", got, "parent")
	}
}
