package errs

import (
	"context"
	"encoding/json"
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
		{name: "invalid model id", err: llamamodels.ErrInvalidModelID, code: InvalidArgument},
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
		errType    string
	}{
		{name: "model not found", err: llamamodels.ErrModelNotFound, statusCode: 404, code: "not_found", errType: "not_found_error"},
		{name: "server busy", err: kronkpool.ErrServerBusy, statusCode: 503, code: "unavailable", errType: "server_error"},
		{name: "admission timeout", err: kronk.ErrAdmissionTimeout, statusCode: 429, code: "resource_exhausted", errType: "rate_limit_error"},
		{name: "invalid messages", err: model.ErrMessagesInvalid, statusCode: 400, code: "invalid_argument", errType: "invalid_request_error"},
		{name: "invalid request", err: model.ErrInvalidRequest, statusCode: 400, code: "invalid_argument", errType: "invalid_request_error"},
		{name: "invalid model id", err: llamamodels.ErrInvalidModelID, statusCode: 400, code: "invalid_argument", errType: "invalid_request_error"},
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

			var got struct {
				Error struct {
					Message string `json:"message"`
					Type    string `json:"type"`
					Code    string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Error.Message != appErr.Message {
				t.Errorf("message: got %q, want %q", got.Error.Message, appErr.Message)
			}
			if got.Error.Type != tt.errType {
				t.Errorf("type: got %q, want %q", got.Error.Type, tt.errType)
			}
			if got.Error.Code != tt.code {
				t.Errorf("code: got %q, want %q", got.Error.Code, tt.code)
			}

			var roundTrip Error
			if err := json.Unmarshal(recorder.Body.Bytes(), &roundTrip); err != nil {
				t.Fatalf("Unmarshal Error: %v", err)
			}
			if !roundTrip.Equal(appErr) {
				t.Errorf("round trip: got %+v, want %+v", roundTrip, *appErr)
			}
		})
	}
}
