package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunWebReturnsTerminalServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"status":"upstream download failed"}`)
	}))
	defer srv.Close()

	t.Setenv("KRONK_WEB_API_HOST", srv.URL)

	err := runWeb(context.Background(), "owner/model", "", "")
	if err == nil || !strings.Contains(err.Error(), "upstream download failed") {
		t.Fatalf("runWeb: got %v, want upstream download failure", err)
	}
}

func TestRunWebCompletes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"status":"downloaded"}`)
	}))
	defer srv.Close()

	t.Setenv("KRONK_WEB_API_HOST", srv.URL)

	if err := runWeb(context.Background(), "owner/model", "", ""); err != nil {
		t.Fatalf("runWeb: %v", err)
	}
}
