package prepare

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDoclingExtractorConvert(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(sourcePath, []byte("pdf-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	var polls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/convert/file/async":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("files")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			if header.Filename != "guide.pdf" || header.Header.Get("Content-Type") != "application/pdf" {
				http.Error(w, "unexpected file metadata", http.StatusBadRequest)
				return
			}
			if got, want := r.MultipartForm.Value["to_formats"], []string{"md", "json"}; !reflect.DeepEqual(got, want) {
				http.Error(w, fmt.Sprintf("to_formats: got %v want %v", got, want), http.StatusBadRequest)
				return
			}
			if r.FormValue("table_mode") != "accurate" || r.FormValue("pdf_backend") != "dlparse_v4" {
				http.Error(w, "missing conversion options", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"task_id":"test-task","task_status":"pending"}`))

		case r.Method == http.MethodGet && r.URL.Path == "/v1/status/poll/test-task":
			polls++
			status := "started"
			if polls == 2 {
				status = "success"
			}
			_, _ = fmt.Fprintf(w, `{"task_id":"test-task","task_status":%q}`, status)

		case r.Method == http.MethodGet && r.URL.Path == "/v1/result/test-task":
			_, _ = w.Write([]byte(`{"document":{"md_content":"# Guide\n\nExtracted text.","json_content":{"texts":[{"text":"Extracted text."}]}},"status":"success","processing_time":1.25}`))

		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	extractor.pollInterval = time.Millisecond

	document, err := extractor.Convert(t.Context(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if document.Markdown != "# Guide\n\nExtracted text." {
		t.Fatalf("markdown: got %q", document.Markdown)
	}
	if string(document.StructuredJSON) != `{"texts":[{"text":"Extracted text."}]}` {
		t.Fatalf("structured JSON: got %q", document.StructuredJSON)
	}
	if document.ProcessingTime != 1.25 {
		t.Fatalf("processing time: got %v", document.ProcessingTime)
	}
}

func TestDoclingExtractorConvertDOCXOmitsPDFBackend(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.docx")
	if err := os.WriteFile(sourcePath, []byte("docx-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/convert/file/async":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, header, err := r.FormFile("files")
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			defer file.Close()
			if header.Header.Get("Content-Type") != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || r.FormValue("pdf_backend") != "" {
				http.Error(w, "unexpected DOCX options", http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"task_id":"docx-task","task_status":"success"}`))
		case "/v1/result/docx-task":
			_, _ = w.Write([]byte(`{"document":{"md_content":"Document","json_content":{"texts":[{"text":"Document"}]}},"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := extractor.Convert(t.Context(), sourcePath); err != nil {
		t.Fatal(err)
	}
}

func TestDoclingExtractorRetriesTransientStatusAndResultReads(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(sourcePath, []byte("pdf-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	var polls, results int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/convert/file/async":
			_, _ = w.Write([]byte(`{"task_id":"task","task_status":"pending"}`))
		case "/v1/status/poll/task":
			polls++
			if polls == 1 {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte(`{"task_id":"task","task_status":"success"}`))
		case "/v1/result/task":
			results++
			if results == 1 {
				http.Error(w, "temporarily unavailable", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(`{"document":{"md_content":"Document","json_content":{"texts":[{"text":"Document"}]}},"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	extractor.pollInterval = 0
	extractor.retryDelay = 0
	if _, err := extractor.Convert(t.Context(), sourcePath); err != nil {
		t.Fatal(err)
	}
	if polls != 2 || results != 2 {
		t.Fatalf("polls = %d, results = %d; want one retry for each transient read", polls, results)
	}
}

func TestDoclingExtractorDoesNotRetryPermanentReadFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "missing", http.StatusNotFound)
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	extractor.retryDelay = 0
	_, err = extractor.result(t.Context(), "missing")
	if err == nil || calls != 1 || !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("error = %v, calls = %d; want one non-retried permanent failure", err, calls)
	}
}

func TestDoclingExtractorRejectsInvalidStructuredJSON(t *testing.T) {
	responses := []string{
		`{"document":{"md_content":"Document"},"status":"success"}`,
		`{"document":{"md_content":"Document","json_content":null},"status":"success"}`,
		`{"document":{"md_content":"Document","json_content":[]},"status":"success"}`,
		`{"document":{"md_content":"Document","json_content":{}},"status":"success"}`,
	}
	for _, response := range responses {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()

			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			_, err = extractor.result(t.Context(), "task")
			if err == nil || !strings.Contains(err.Error(), "invalid structured JSON") {
				t.Fatalf("error = %v, want invalid structured JSON", err)
			}
		})
	}
}

func TestDoclingExtractorFailures(t *testing.T) {
	tests := []struct {
		name       string
		sourceName string
		response   string
		status     int
		want       string
	}{
		{name: "unsupported file", sourceName: "records.csv", want: "unsupported file type"},
		{name: "server error", sourceName: "guide.pdf", status: http.StatusServiceUnavailable, response: "warming up", want: "503 Service Unavailable"},
		{name: "missing task id", sourceName: "guide.pdf", response: `{"task_status":"pending"}`, want: "empty task id"},
		{name: "failed task", sourceName: "guide.pdf", response: `{"task_id":"task","task_status":"failure"}`, want: "task failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				status := test.status
				if status == 0 {
					status = http.StatusOK
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			extractor, err := NewDoclingExtractor(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(t.TempDir(), test.sourceName)
			if err := os.WriteFile(sourcePath, []byte("content"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err = extractor.Convert(t.Context(), sourcePath)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDoclingExtractorHonorsCancellation(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(sourcePath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"task_id":"task","task_status":"pending"}`))
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = extractor.Convert(ctx, sourcePath)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: got %v, want context canceled", err)
	}
}

func TestDoclingExtractorExplainsUnavailableService(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(sourcePath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	client := server.Client()
	server.Close()

	extractor, err := NewDoclingExtractor(endpoint, client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = extractor.Convert(t.Context(), sourcePath)
	if err == nil || !strings.Contains(err.Error(), "make docling-up") {
		t.Fatalf("error: got %v, want make docling-up guidance", err)
	}
}

func TestDoclingExtractorRejectsOversizedResponse(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "guide.pdf")
	if err := os.WriteFile(sourcePath, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "268435457")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	extractor, err := NewDoclingExtractor(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	_, err = extractor.Convert(t.Context(), sourcePath)
	if err == nil || !strings.Contains(err.Error(), "response exceeds") {
		t.Fatalf("error: got %v, want response-size rejection", err)
	}
}
