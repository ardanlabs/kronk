package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type rawErrorResponse struct{}

func (rawErrorResponse) Encode() ([]byte, string, error) {
	data, err := json.Marshal(map[string]string{"error": "recovered"})
	return data, "application/json", err
}

func (rawErrorResponse) HTTPStatus() int {
	return http.StatusInternalServerError
}

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

func TestRawHandlerFuncWritesMiddlewareResponse(t *testing.T) {
	recoverPanic := func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, r *http.Request) (resp Encoder) {
			defer func() {
				if recover() != nil {
					resp = rawErrorResponse{}
				}
			}()
			return next(ctx, r)
		}
	}

	app := NewApp(func(context.Context, string, ...any) {}, recoverPanic)
	app.RawHandlerFunc(http.MethodGet, "", "/raw", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	rr := httptest.NewRecorder()
	app.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/raw", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", rr.Code, http.StatusInternalServerError)
	}
	if got := rr.Body.String(); got != "{\"error\":\"recovered\"}" {
		t.Errorf("body: got %q, want recovered error", got)
	}
}
