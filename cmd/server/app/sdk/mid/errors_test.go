package mid

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestErrorsMasksInternalMessages(t *testing.T) {
	tests := []struct {
		name string
		code errs.ErrCode
	}{
		{name: "internal", code: errs.Internal},
		{name: "internal only log", code: errs.InternalOnlyLog},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logger.New(io.Discard, logger.LevelError, "TEST", nil)
			handler := Errors(log)(func(context.Context, *http.Request) web.Encoder {
				return errs.Errorf(tt.code, "sensitive /path/to/database")
			})

			resp := handler(t.Context(), httptest.NewRequest("GET", "/", nil))
			appErr, ok := resp.(*errs.Error)
			if !ok {
				t.Fatalf("response: got %T, want *errs.Error", resp)
			}
			if got, want := appErr.Message, "Internal Server Error"; got != want {
				t.Errorf("message: got %q, want %q", got, want)
			}
		})
	}
}
