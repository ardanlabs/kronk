package mid

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestLoggerReportsCommittedResponseError(t *testing.T) {
	var completed logger.Record
	var output bytes.Buffer
	log := logger.NewWithEvents(&output, logger.LevelInfo, "TEST", nil, logger.Events{
		Info: func(_ context.Context, record logger.Record) {
			if record.Message == "request completed" {
				completed = record
			}
		},
	})

	wantErr := errs.FromSDK(context.Canceled)
	handler := Logger(log)(func(context.Context, *http.Request) web.Encoder {
		return web.NewNoResponseError(wantErr)
	})

	resp := handler(t.Context(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if got := checkIsError(resp); got != wantErr {
		t.Errorf("response error: got %v, want %v", got, wantErr)
	}

	gotCode, ok := completed.Attributes["statuscode"].(errs.ErrCode)
	if !ok {
		t.Fatalf("statuscode: got %T, want errs.ErrCode", completed.Attributes["statuscode"])
	}
	if !gotCode.Equal(errs.Canceled) {
		t.Errorf("statuscode: got %s, want %s", gotCode, errs.Canceled)
	}
}

func TestErrorsPreservesCommittedResponse(t *testing.T) {
	var output bytes.Buffer
	log := logger.New(&output, logger.LevelError, "TEST", nil)
	wantErr := errs.FromSDK(context.Canceled)
	handler := Errors(log)(func(context.Context, *http.Request) web.Encoder {
		return web.NewNoResponseError(wantErr)
	})

	resp := handler(t.Context(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if _, ok := resp.(web.NoResponse); !ok {
		t.Fatalf("response: got %T, want web.NoResponse", resp)
	}
	if got := checkIsError(resp); got != wantErr {
		t.Errorf("response error: got %v, want %v", got, wantErr)
	}

	w := responseWriter{}
	if err := web.Respond(t.Context(), &w, resp); err != nil {
		t.Fatalf("Respond: got %v, want nil", err)
	}
	if w.wrote {
		t.Fatal("Respond wrote a second response")
	}
}

type responseWriter struct {
	header http.Header
	wrote  bool
}

func (w *responseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *responseWriter) Write(data []byte) (int, error) {
	w.wrote = true
	return len(data), nil
}

func (w *responseWriter) WriteHeader(int) {
	w.wrote = true
}
