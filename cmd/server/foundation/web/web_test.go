package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotFoundHandlerUsesApplicationMiddleware(t *testing.T) {
	var middlewareCalls int
	var traceID string

	mw := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, r *http.Request) Encoder {
			middlewareCalls++
			traceID = GetTraceID(ctx)
			return next(ctx, r)
		}
	}

	app := NewApp(func(context.Context, string, ...any) {}, mw)
	app.NotFoundHandler()

	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v1/completions", nil))

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusNotFound)
	}
	if got := rr.Body.String(); got != "Not Found\n" {
		t.Errorf("body: got %q, want %q", got, "Not Found\n")
	}
	if middlewareCalls != 1 {
		t.Errorf("middleware calls: got %d, want 1", middlewareCalls)
	}
	if traceID == defaultTraceID {
		t.Errorf("trace ID: got %q, want non-default value", traceID)
	}
}
