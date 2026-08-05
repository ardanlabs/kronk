package downloader

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadResumesPartialFile(t *testing.T) {
	content := []byte("complete model content")
	partial := content[:8]
	rangeCh := make(chan string, 1)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", fmt.Sprint(len(content)))
		if r.Method == http.MethodHead {
			return
		}

		gotRange := r.Header.Get("Range")
		rangeCh <- gotRange
		if gotRange != "" {
			w.Header().Set("Content-Length", fmt.Sprint(len(content)-len(partial)))
			w.WriteHeader(http.StatusPartialContent)
			if _, err := w.Write(content[len(partial):]); err != nil {
				t.Errorf("write partial response: %v", err)
			}
			return
		}

		if _, err := w.Write(content); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer srv.Close()

	t.Setenv("KRONK_SKIP_NETWORK_CHECK", "yes")
	dest := t.TempDir()
	modelFile := filepath.Join(dest, "model.gguf")
	if err := os.WriteFile(modelFile, partial, 0o600); err != nil {
		t.Fatalf("write partial file: %v", err)
	}

	if _, err := Download(context.Background(), srv.URL+"/model.gguf", dest, nil, SizeIntervalMB); err != nil {
		t.Fatalf("Download: %v", err)
	}

	gotRange := <-rangeCh
	if gotRange != "bytes=8-" {
		t.Errorf("Range: got %q, want %q", gotRange, "bytes=8-")
	}

	file, err := os.Open(modelFile)
	if err != nil {
		t.Fatalf("open model: %v", err)
	}
	defer file.Close()

	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("read model: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("model content: got %q, want %q", got, content)
	}
}
