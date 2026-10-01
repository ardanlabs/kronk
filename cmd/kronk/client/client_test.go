package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientDoNilResponseTarget(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "response does not need to be decoded")
	}))
	defer srv.Close()

	cln := New(NoopLogger, WithClient(srv.Client()))
	if err := cln.Do(t.Context(), http.MethodGet, srv.URL, nil, nil); err != nil {
		t.Fatalf("Do: %v", err)
	}
}

func TestSSEClientDoWithErrors(t *testing.T) {
	tests := []struct {
		name    string
		stream  string
		want    string
		wantErr string
	}{
		{
			name:   "complete stream",
			stream: ": keep-alive\nevent: progress\nid: 1\ndata: {\"status\":\"downloaded\"}\ndata: [DONE]\n",
			want:   "downloaded",
		},
		{
			name:   "short and unknown fields are ignored",
			stream: "x\nevent:\nretry: 1000\ndata:{\"status\":\"downloaded\"}\n",
			want:   "downloaded",
		},
		{
			name:    "malformed event",
			stream:  "data: {\n",
			wantErr: "decoding SSE response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, tt.stream)
			}))
			defer srv.Close()

			cln := NewSSE[struct {
				Status string `json:"status"`
			}](NoopLogger, WithClient(srv.Client()))

			ch := make(chan struct {
				Status string `json:"status"`
			})
			errCh, err := cln.DoWithErrors(context.Background(), http.MethodGet, srv.URL, nil, ch)
			if err != nil {
				t.Fatalf("DoWithErrors: %v", err)
			}

			var got string
			for event := range ch {
				got = event.Status
			}

			streamErr := <-errCh
			if tt.wantErr != "" {
				if streamErr == nil || !strings.Contains(streamErr.Error(), tt.wantErr) {
					t.Fatalf("stream error: got %v, want containing %q", streamErr, tt.wantErr)
				}
				return
			}

			if streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if got != tt.want {
				t.Errorf("status: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSSEClientDoWithErrorsCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"status":"started"}`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()

	type event struct {
		Status string `json:"status"`
	}

	ctx, cancel := context.WithCancel(context.Background())
	cln := NewSSE[event](NoopLogger, WithClient(srv.Client()))
	ch := make(chan event)
	errCh, err := cln.DoWithErrors(ctx, http.MethodGet, srv.URL, nil, ch)
	if err != nil {
		t.Fatalf("DoWithErrors: %v", err)
	}

	got := <-ch
	if got.Status != "started" {
		t.Fatalf("status: got %q, want %q", got.Status, "started")
	}
	cancel()

	for range ch {
	}
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Errorf("stream error: got %v, want %v", err, context.Canceled)
	}
}
