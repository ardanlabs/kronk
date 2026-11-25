package pull

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
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

	err := runWeb(context.Background(), "owner/model", "", models.CatalogDraft{})
	if err == nil || !strings.Contains(err.Error(), "upstream download failed") {
		t.Fatalf("runWeb: got %v, want upstream download failure", err)
	}
}

func TestRunWebCompletes(t *testing.T) {
	draft := models.CatalogDraft{Method: models.DraftMethodDSpark, URL: "https://huggingface.co/other/repo/resolve/main/dspark-head.gguf_file"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req toolapp.PullRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req.ModelURL != "owner/model" || req.ProjURL != "projection" || req.Draft != draft {
			t.Errorf("request: got %+v, want model, projection and DSpark override", req)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintln(w, `data: {"status":"downloaded"}`)
	}))
	defer srv.Close()

	t.Setenv("KRONK_WEB_API_HOST", srv.URL)

	if err := runWeb(context.Background(), "owner/model", "projection", draft); err != nil {
		t.Fatalf("runWeb: %v", err)
	}
}

func TestDraftFlags(t *testing.T) {
	for _, tt := range []struct {
		name, url, method string
	}{
		{"missing method", "https://huggingface.co/other/repo/resolve/main/head.gguf", ""},
		{"missing URL", "", "dspark"},
		{"unknown method", "https://huggingface.co/other/repo/resolve/main/head.gguf", "unknown"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("local", false, "")
			cmd.Flags().String("proj", "", "")
			cmd.Flags().String("draft", tt.url, "")
			cmd.Flags().String("draft-method", tt.method, "")
			if err := run(cmd, []string{"owner/model"}); err == nil {
				t.Fatal("invalid draft flags accepted")
			}
		})
	}
}
