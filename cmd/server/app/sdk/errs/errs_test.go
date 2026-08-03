package errs

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/bucky/pkg/audio"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	buckyffmpeg "github.com/ardanlabs/kronk/sdk/bucky/ffmpeg"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	kronkpool "github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/pool/engine/resman"
	buckylibs "github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/ardanlabs/kronk/sdk/tools/github"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	llamamodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestFromSDK(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code ErrCode
	}{
		{name: "admission timeout", err: kronk.ErrAdmissionTimeout, code: ResourceExhausted},
		{name: "context canceled", err: context.Canceled, code: Canceled},
		{name: "context deadline", err: context.DeadlineExceeded, code: DeadlineExceeded},
		{name: "file inputs unsupported", err: model.ErrFileInputsUnsupported, code: InvalidArgument},
		{name: "messages missing", err: model.ErrMessagesMissing, code: InvalidArgument},
		{name: "messages invalid", err: model.ErrMessagesInvalid, code: InvalidArgument},
		{name: "invalid request", err: model.ErrInvalidRequest, code: InvalidArgument},
		{name: "llama model not found", err: llamamodels.ErrModelNotFound, code: NotFound},
		{name: "bucky model not found", err: buckymodels.ErrModelNotFound, code: NotFound},
		{name: "server busy", err: kronkpool.ErrServerBusy, code: Unavailable},
		{name: "pool no capacity", err: kronkpool.ErrNoCapacity, code: ResourceExhausted},
		{name: "resource manager no capacity", err: resman.ErrNoCapacity, code: ResourceExhausted},
		{name: "hugging face not found", err: hf.ErrNotFound, code: NotFound},
		{name: "hugging face throttled", err: hf.ErrThrottled, code: TooManyRequests},
		{name: "github rate limited", err: github.ErrRateLimited, code: TooManyRequests},
		{name: "ffmpeg not installed", err: buckyffmpeg.ErrNotInstalled, code: FailedPrecondition},
		{name: "unsupported audio", err: audio.ErrUnsupportedFormat, code: InvalidArgument},
		{name: "llama libraries read only", err: libs.ErrReadOnly, code: FailedPrecondition},
		{name: "bucky libraries read only", err: buckylibs.ErrReadOnly, code: FailedPrecondition},
		{name: "unknown", err: errors.New("unknown"), code: Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("wrapped: %w", tt.err)
			got := FromSDK(err)

			if !got.Code.Equal(tt.code) {
				t.Errorf("Code: got %s, want %s", got.Code, tt.code)
			}
			if got.Message != err.Error() {
				t.Errorf("Message: got %q, want %q", got.Message, err)
			}
			if !strings.Contains(got.FuncName, "TestFromSDK") {
				t.Errorf("FuncName: got %q, want TestFromSDK caller", got.FuncName)
			}
			if !strings.Contains(got.FileName, "errs_test.go:") {
				t.Errorf("FileName: got %q, want errs_test.go caller", got.FileName)
			}
		})
	}
}

func TestFromSDKHTTPResponse(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		code       string
	}{
		{name: "model not found", err: llamamodels.ErrModelNotFound, statusCode: 404, code: "not_found"},
		{name: "server busy", err: kronkpool.ErrServerBusy, statusCode: 503, code: "unavailable"},
		{name: "admission timeout", err: kronk.ErrAdmissionTimeout, statusCode: 429, code: "resource_exhausted"},
		{name: "invalid messages", err: model.ErrMessagesInvalid, statusCode: 400, code: "invalid_argument"},
		{name: "invalid request", err: model.ErrInvalidRequest, statusCode: 400, code: "invalid_argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			appErr := FromSDK(fmt.Errorf("request failed: %w", tt.err))

			if err := web.Respond(t.Context(), recorder, appErr); err != nil {
				t.Fatalf("Respond: %v", err)
			}

			if recorder.Code != tt.statusCode {
				t.Errorf("status: got %d, want %d", recorder.Code, tt.statusCode)
			}
			if !strings.Contains(recorder.Body.String(), fmt.Sprintf(`"code":%q`, tt.code)) {
				t.Errorf("body: got %q, want code %q", recorder.Body.String(), tt.code)
			}
		})
	}
}
