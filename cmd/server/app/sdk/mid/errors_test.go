package mid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestErrorsIncompleteModel(t *testing.T) {
	modelPath := filepath.Join(t.TempDir(), "model.gguf")
	if err := os.WriteFile(modelPath, []byte("partial"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, _, err := model.VerifyArtifact(modelPath, model.ArtifactDigest{
		SHA256: strings.Repeat("a", 64),
		Size:   100,
	}, nil, false)
	if err == nil {
		t.Fatal("VerifyArtifact: got nil, want size mismatch")
	}

	var logs bytes.Buffer
	log := logger.New(&logs, logger.LevelError, "TEST", nil)
	handler := Errors(log)(func(context.Context, *http.Request) web.Encoder {
		return errs.FromSDK(fmt.Errorf("acquire-model: validate-config: %w", err))
	})
	resp := handler(t.Context(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	recorder := httptest.NewRecorder()
	if err := web.Respond(t.Context(), recorder, resp); err != nil {
		t.Fatalf("Respond: %v", err)
	}
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	var got errs.Error
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.Code.Equal(errs.FailedPrecondition) {
		t.Errorf("code: got %s, want %s", got.Code, errs.FailedPrecondition)
	}
	for _, want := range []string{"size mismatch", "expected 100, got 7", "wait for any active pull", "kronk model pull"} {
		if !strings.Contains(got.Message, want) {
			t.Errorf("message: got %q, want containing %q", got.Message, want)
		}
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs: got %q, want containing %q", logs.String(), want)
		}
	}
}

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

func TestMessagesErrorWireProtocol(t *testing.T) {
	for _, tt := range []struct {
		name     string
		code     errs.ErrCode
		status   int
		wantType string
	}{
		{"validation", errs.InvalidArgument, 400, "invalid_request_error"},
		{"authentication", errs.Unauthenticated, 401, "authentication_error"},
		{"permission", errs.PermissionDenied, 403, "permission_error"},
		{"model missing", errs.NotFound, 404, "not_found_error"},
		{"rate limit", errs.TooManyRequests, 429, "rate_limit_error"},
		{"capacity", errs.Unavailable, 503, "overloaded_error"},
		{"internal", errs.Internal, 500, "api_error"},
		{"panic", errs.InternalOnlyLog, 500, "api_error"},
		{"deadline", errs.DeadlineExceeded, 504, "api_error"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := logger.New(io.Discard, logger.LevelError, "TEST", nil)
			err := errs.Errorf(tt.code, "request failed")
			handler := Errors(log)(func(context.Context, *http.Request) web.Encoder { return err })
			resp := handler(t.Context(), httptest.NewRequest("POST", "/v1/messages", nil))
			if appErr, ok := errors.AsType[*errs.Error](checkIsError(resp)); !ok || !appErr.Code.Equal(tt.code) && !tt.code.Equal(errs.InternalOnlyLog) {
				t.Fatalf("error identity: got %v, want %s", appErr, tt.code)
			}
			rr := httptest.NewRecorder()
			if err := web.Respond(t.Context(), rr, resp); err != nil {
				t.Fatal(err)
			}
			var body struct {
				Type  string `json:"type"`
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if rr.Code != tt.status || body.Type != "error" || body.Error.Type != tt.wantType {
				t.Errorf("wire response: got %d %s, want %d error/%s", rr.Code, rr.Body, tt.status, tt.wantType)
			}
			wantMessage := "request failed"
			if tt.code.Equal(errs.Internal) || tt.code.Equal(errs.InternalOnlyLog) {
				wantMessage = "Internal Server Error"
			}
			if body.Error.Message != wantMessage {
				t.Errorf("message: got %q, want %q", body.Error.Message, wantMessage)
			}
		})
	}
}

func TestMessagesErrorsDoNotRewriteCommittedStreams(t *testing.T) {
	log := logger.New(io.Discard, logger.LevelError, "TEST", nil)
	handler := Errors(log)(func(context.Context, *http.Request) web.Encoder {
		return web.NewNoResponseError(errs.Errorf(errs.Internal, "write failed"))
	})
	rr := httptest.NewRecorder()
	resp := handler(t.Context(), httptest.NewRequest("POST", "/v1/messages", nil))
	if err := web.Respond(t.Context(), rr, resp); err != nil {
		t.Fatal(err)
	}
	if rr.Body.Len() != 0 || checkIsError(resp) == nil {
		t.Fatalf("committed stream: got %q, error %v", rr.Body, checkIsError(resp))
	}
}
