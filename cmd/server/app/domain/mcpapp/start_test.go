package mcpapp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthenticate(t *testing.T) {
	log := logger.New(io.Discard, logger.LevelInfo, "TEST", func(context.Context) string { return "" })

	tests := []struct {
		name         string
		token        string
		authenticate func(context.Context, string) error
		wantStatus   int
		wantCalled   bool
	}{
		{
			name:       "disabled",
			wantStatus: http.StatusNoContent,
			wantCalled: true,
		},
		{
			name:  "authorized",
			token: "Bearer token",
			authenticate: func(_ context.Context, token string) error {
				if token != "Bearer token" {
					t.Fatalf("token: got %q, want %q", token, "Bearer token")
				}
				return nil
			},
			wantStatus: http.StatusNoContent,
			wantCalled: true,
		},
		{
			name: "unauthenticated",
			authenticate: func(_ context.Context, _ string) error {
				return status.Error(codes.Unauthenticated, "authentication failed")
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "permission denied",
			authenticate: func(_ context.Context, _ string) error {
				return status.Error(codes.PermissionDenied, "permission denied")
			},
			wantStatus: http.StatusForbidden,
		},
		{
			name: "rate limited",
			authenticate: func(_ context.Context, _ string) error {
				return status.Error(codes.ResourceExhausted, "rate limited")
			},
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name: "unavailable",
			authenticate: func(_ context.Context, _ string) error {
				return status.Error(codes.Unavailable, "unavailable")
			},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name: "deadline",
			authenticate: func(_ context.Context, _ string) error {
				return status.Error(codes.DeadlineExceeded, "deadline")
			},
			wantStatus: http.StatusGatewayTimeout,
		},
		{
			name: "internal",
			authenticate: func(_ context.Context, _ string) error {
				return errors.New("authentication failed")
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})

			handler := authenticate(Config{Log: log, Authenticate: tt.authenticate}, next)
			req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
			req.Header.Set("Authorization", tt.token)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status: got %d, want %d", rec.Code, tt.wantStatus)
			}
			if called != tt.wantCalled {
				t.Errorf("called: got %t, want %t", called, tt.wantCalled)
			}
			if got := rec.Header().Get("WWW-Authenticate"); (tt.wantStatus == http.StatusUnauthorized) != (got == "Bearer") {
				t.Errorf("WWW-Authenticate: got %q for status %d", got, tt.wantStatus)
			}
		})
	}
}
