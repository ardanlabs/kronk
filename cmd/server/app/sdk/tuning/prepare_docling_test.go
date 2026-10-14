package tuning

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoclingConversion(t *testing.T) {
	for _, ext := range []string{".pdf", ".docx"} {
		t.Run(ext, func(t *testing.T) {
			var temporary atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/convert/file/async":
					if err := r.ParseMultipartForm(1 << 20); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					defer r.MultipartForm.RemoveAll()
					if r.Method != "POST" || !slices.Equal(r.MultipartForm.Value["to_formats"], []string{"md", "json"}) || r.FormValue("table_mode") != "accurate" {
						t.Error("incorrect conversion settings")
					}
					if (r.FormValue("pdf_backend") == "dlparse_v4") != (ext == ".pdf") {
						t.Error("wrong PDF backend option")
					}
					file, header, err := r.FormFile("files")
					if err != nil {
						t.Error(err)
						return
					}
					defer file.Close()
					data, err := io.ReadAll(file)
					if err != nil || string(data) != "document fixture" || header.Filename != "source"+ext {
						t.Errorf("uploaded document: %s %+v %v", data, header, err)
					}
					fmt.Fprint(w, `{"task_id":"task-1","task_status":"pending"}`)
				case "/v1/status/poll/task-1":
					fmt.Fprint(w, `{"task_id":"task-1","task_status":"success"}`)
				case "/v1/result/task-1":
					if !temporary.Swap(true) {
						w.WriteHeader(503)
						return
					}
					fmt.Fprint(w, `{"status":"success","processing_time":1.25,"document":{"md_content":"Extracted facts.","json_content":{"texts":[{"text":"Extracted facts."}]}}}`)
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source"+ext)
			if err := os.WriteFile(path, []byte("document fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := extractor.Convert(t.Context(), path)
			if err != nil || document.Markdown != "Extracted facts." || document.ProcessingTime != 1.25 || string(document.StructuredJSON) != `{"texts":[{"text":"Extracted facts."}]}` {
				t.Fatalf("conversion result: %+v %v", document, err)
			}
		})
	}
}

func TestDoclingCancellationAndInvalidResult(t *testing.T) {
	for _, mode := range []string{"pending", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					io.Copy(io.Discard, r.Body)
					if mode == "pending" {
						fmt.Fprint(w, `{"task_id":"task-1","task_status":"pending"}`)
					} else {
						fmt.Fprint(w, `{"task_id":"task-1","task_status":"success"}`)
					}
					return
				}
				fmt.Fprint(w, `{"status":"success","document":{"md_content":"Missing structured content","json_content":{}}}`)
			}))
			defer server.Close()
			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source.pdf")
			if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
				t.Fatal(err)
			}
			timeout := 5 * time.Second
			if mode == "pending" {
				timeout = 100 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(t.Context(), timeout)
			defer cancel()
			_, err = extractor.Convert(ctx, path)
			if err == nil || mode == "pending" && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("%s result: %v", mode, err)
			}
			if mode == "invalid" && (errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "invalid or empty document content")) {
				t.Fatalf("invalid content was not rejected: %v", err)
			}
		})
	}
}
