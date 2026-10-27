package libs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebInstallCompletion(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		wantErr string
	}{
		{name: "downloaded", stream: "data: {\"status\":\"downloaded\"}\n\n"},
		{name: "server failure", stream: "data: {\"status\":\"download failed\"}\n\n", wantErr: "download failed"},
		{name: "progress only", stream: "data: {\"status\":\"downloading\"}\n\n", wantErr: "did not complete successfully"},
		{name: "empty stream", wantErr: "did not complete successfully"},
		{name: "malformed event after success", stream: "data: {\"status\":\"downloaded\"}\n\ndata: {\n\n", wantErr: "decoding SSE response"},
	}

	for _, triple := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("triple=%t/%s", triple, tt.name), func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprint(w, tt.stream)
				}))
				defer srv.Close()
				t.Setenv("KRONK_WEB_API_HOST", srv.URL)

				var err error
				if triple {
					err = webPullInstall(t.Context(), installOpts{arch: "arm64", os: "darwin", processor: "metal"})
				} else {
					err = runWeb(t.Context(), false, "")
				}
				if tt.wantErr == "" {
					if err != nil {
						t.Fatalf("install: got %v, want nil", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("install: got %v, want error containing %q", err, tt.wantErr)
				}
			})
		}
	}

	t.Run("already current", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"status\":\"using installed version\"}\n\n")
		}))
		defer srv.Close()
		t.Setenv("KRONK_WEB_API_HOST", srv.URL)
		if err := runWeb(t.Context(), false, ""); err != nil {
			t.Fatalf("install: got %v, want nil", err)
		}
	})

	t.Run("canceled command context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if err := runWeb(ctx, false, ""); !errors.Is(err, context.Canceled) {
			t.Fatalf("install: got %v, want %v", err, context.Canceled)
		}
	})
}
