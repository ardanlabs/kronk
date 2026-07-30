package authapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/rate"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestAuthenticateDisabledModes(t *testing.T) {
	log := logger.New(io.Discard, logger.LevelInfo, "TEST", func(context.Context) string { return "" })
	req := AuthenticateRequest_builder{Admin: new(true)}.Build()

	t.Run("all authentication disabled", func(t *testing.T) {
		app := newApp(Config{Log: log})
		if _, err := app.Authenticate(context.Background(), req); err != nil {
			t.Fatalf("Authenticate: got error %v, want nil", err)
		}
	})

	t.Run("admin authentication enabled", func(t *testing.T) {
		app := newApp(Config{Log: log, AdminAuthEnabled: true})
		if _, err := app.Authenticate(context.Background(), req); err == nil {
			t.Fatal("Authenticate: got nil error, want missing metadata error")
		}
	})
}

func TestAuthenticationError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "unauthenticated", err: security.ErrUnauthenticated, code: codes.Unauthenticated},
		{name: "permission denied", err: auth.ErrForbidden, code: codes.PermissionDenied},
		{name: "rate limited", err: rate.ErrRateLimitExceeded, code: codes.ResourceExhausted},
		{name: "internal", err: errors.New("database unavailable"), code: codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := authenticationError(fmt.Errorf("wrapped: %w", tt.err))
			if got := status.Code(err); got != tt.code {
				t.Errorf("code: got %s, want %s", got, tt.code)
			}
		})
	}
}
