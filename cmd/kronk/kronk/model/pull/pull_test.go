package pull

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
)

func TestProgressPrinterPreventsWrapping(t *testing.T) {
	var buf bytes.Buffer
	prt := progressPrinter{w: &buf, rewrite: true}

	prt.print(toolapp.PullResponse{
		Status:   "first progress update long enough to wrap",
		Progress: &toolapp.PullProgress{Src: "model.gguf"},
	})
	prt.print(toolapp.PullResponse{
		Status:   "second progress update",
		Progress: &toolapp.PullProgress{Src: "model.gguf"},
	})
	prt.close()

	want := "\r\x1b[2K\x1b[?7lfirst progress update long enough to wrap\x1b[?7h" +
		"\r\x1b[2K\x1b[?7lsecond progress update\x1b[?7h\n"
	if got := buf.String(); got != want {
		t.Errorf("output: got %q, want %q", got, want)
	}
}

func TestProgressPrinterPreservesCompletedFiles(t *testing.T) {
	var buf bytes.Buffer
	prt := progressPrinter{w: &buf, rewrite: true}

	prt.print(toolapp.PullResponse{
		Status:   "model progress",
		Progress: &toolapp.PullProgress{Src: "model.gguf"},
	})
	prt.print(toolapp.PullResponse{
		Status:   "projection progress",
		Progress: &toolapp.PullProgress{Src: "projection.gguf", Complete: true},
	})
	prt.print(toolapp.PullResponse{Status: "downloaded"})

	want := "\r\x1b[2K\x1b[?7lmodel progress\x1b[?7h\n" +
		"\r\x1b[2K\x1b[?7lprojection progress\x1b[?7h\n" +
		"downloaded\n"
	if got := buf.String(); got != want {
		t.Errorf("output: got %q, want %q", got, want)
	}
}

func TestProgressPrinterDoesNotRewriteRedirectedOutput(t *testing.T) {
	var buf bytes.Buffer
	prt := progressPrinter{w: &buf}

	prt.print(toolapp.PullResponse{
		Status:   "first progress update",
		Progress: &toolapp.PullProgress{Src: "model.gguf"},
	})
	prt.print(toolapp.PullResponse{
		Status:   "second progress update",
		Progress: &toolapp.PullProgress{Src: "model.gguf"},
	})

	want := "first progress update\nsecond progress update\n"
	if got := buf.String(); got != want {
		t.Errorf("output: got %q, want %q", got, want)
	}
}

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
