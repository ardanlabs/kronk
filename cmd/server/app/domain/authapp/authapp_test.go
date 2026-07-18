package authapp

import (
	"context"
	"io"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
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
