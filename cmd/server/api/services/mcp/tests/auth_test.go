package mcpapi_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/mcpapp"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func Test_API_Authenticate(t *testing.T) {
	for _, tc := range []struct {
		name, token string
		disabled    bool
		err         error
		wantStatus  int
	}{
		{name: "disabled", disabled: true, wantStatus: http.StatusOK},
		{name: "authorized", token: "Bearer token", wantStatus: http.StatusOK},
		{name: "unauthenticated", err: status.Error(codes.Unauthenticated, "authentication failed"), wantStatus: http.StatusUnauthorized},
		{name: "permission denied", err: status.Error(codes.PermissionDenied, "permission denied"), wantStatus: http.StatusForbidden},
		{name: "rate limited", err: status.Error(codes.ResourceExhausted, "rate limited"), wantStatus: http.StatusTooManyRequests},
		{name: "unavailable", err: status.Error(codes.Unavailable, "unavailable"), wantStatus: http.StatusServiceUnavailable},
		{name: "deadline", err: status.Error(codes.DeadlineExceeded, "deadline"), wantStatus: http.StatusGatewayTimeout},
		{name: "internal", err: errors.New("authentication failed"), wantStatus: http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			cfg := mcpapp.Config{
				Log:      logger.New(io.Discard, logger.LevelInfo, "TEST", nil),
				Listener: listener,
			}
			if !tc.disabled {
				cfg.Authenticate = func(_ context.Context, token string) error {
					if token != tc.token {
						return errors.New("authorization header was not forwarded")
					}
					return tc.err
				}
			}
			app := mcpapp.Start(t.Context(), cfg)
			t.Cleanup(func() { app.Shutdown(context.Background()) })

			body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"kronk-api-test","version":"1"}}}`
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+listener.Addr().String()+"/mcp", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", tc.token)
			client := &http.Client{Timeout: 5 * time.Second}
			t.Cleanup(client.CloseIdleConnections)
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tc.wantStatus {
				t.Errorf("status: got %d, want %d; body=%s", resp.StatusCode, tc.wantStatus, data)
			}
			if got := resp.Header.Get("WWW-Authenticate"); (tc.wantStatus == http.StatusUnauthorized) != (got == "Bearer") {
				t.Errorf("WWW-Authenticate: got %q for status %d", got, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusOK && !strings.Contains(string(data), `"serverInfo"`) {
				t.Errorf("authorized request did not initialize MCP: %s", data)
			}
		})
	}
}
