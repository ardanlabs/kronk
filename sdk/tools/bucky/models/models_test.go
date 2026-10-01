package models

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestCatalogTinyEnglish(t *testing.T) {
	entry, ok := Catalog()["tiny.en"]
	if !ok {
		t.Fatal("Catalog: tiny.en is missing")
	}

	const wantURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-tiny.en.bin"
	if entry.URL != wantURL {
		t.Errorf("URL: got %q, want %q", entry.URL, wantURL)
	}
}

func TestFullPathNotFound(t *testing.T) {
	var m Models

	_, err := m.FullPath("missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("FullPath: got %v, want %v", err, ErrModelNotFound)
	}
}

func TestCatalogHeaderNotFound(t *testing.T) {
	var m Models

	_, err := m.CatalogHeader(t.Context(), "missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("CatalogHeader: got %v, want %v", err, ErrModelNotFound)
	}
}

func TestCatalogHeaderInstalledModelOutsideCatalog(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	data := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(data, ggmlFileMagic)
	binary.LittleEndian.PutUint32(data[4:], 51864)
	binary.LittleEndian.PutUint32(data[20:], 4)

	modelFile := filepath.Join(m.Path(), "ggml-custom.bin")
	if err := os.WriteFile(modelFile, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	log := func(context.Context, string, ...any) {}
	if err := m.BuildIndex(log, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}

	header, err := m.CatalogHeader(t.Context(), "custom")
	if err != nil {
		t.Fatalf("CatalogHeader: %v", err)
	}
	if got, want := header.ModelType(), "tiny"; got != want {
		t.Errorf("ModelType: got %q, want %q", got, want)
	}
	if header.IsMultilingual() {
		t.Error("IsMultilingual: got true, want false")
	}
}

func TestDownloadResumesPartialModel(t *testing.T) {
	contents := []byte("complete whisper model contents")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", fmt.Sprint(len(contents)))
			return
		}

		start := 0
		if value := r.Header.Get("Range"); value != "" {
			if _, err := fmt.Sscanf(value, "bytes=%d-", &start); err != nil {
				t.Errorf("Range header: %q", value)
				http.Error(w, "bad range", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Length", fmt.Sprint(len(contents)-start))
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(contents)-1, len(contents)))
			w.WriteHeader(http.StatusPartialContent)
		} else {
			w.Header().Set("Content-Length", fmt.Sprint(len(contents)))
		}
		_, _ = w.Write(contents[start:])
	}))
	defer server.Close()

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	stagingDir := filepath.Join(m.Path(), partialDir)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("MkdirAll staging: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "ggml-resume.bin"), contents[:7], 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, err := m.Download(t.Context(), func(context.Context, string, ...any) {}, server.URL+"/ggml-resume.bin"); err != nil {
		t.Fatalf("Download: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(m.Path(), "ggml-resume.bin"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != string(contents) {
		t.Fatalf("downloaded contents: got %q, want %q", got, contents)
	}
}

func TestInterruptedDownloadIsNotInstalled(t *testing.T) {
	contents := []byte("partial whisper model")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(contents)*2))
		if r.Method == http.MethodHead {
			return
		}
		_, _ = w.Write(contents)
	}))
	defer server.Close()

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}
	log := func(context.Context, string, ...any) {}

	if _, err := m.Download(t.Context(), log, server.URL+"/ggml-interrupted.bin"); err == nil {
		t.Fatal("Download: got nil error, want interrupted download error")
	}
	if _, err := os.Stat(filepath.Join(m.Path(), "ggml-interrupted.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("installed model: got %v, want os.ErrNotExist", err)
	}
	if _, err := os.Stat(filepath.Join(m.Path(), partialDir, "ggml-interrupted.bin")); err != nil {
		t.Fatalf("partial model was not retained for resume: %v", err)
	}
	if err := m.BuildIndex(log, false); err != nil {
		t.Fatalf("BuildIndex: %v", err)
	}
	if _, err := m.FullPath("interrupted"); !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("FullPath: got %v, want ErrModelNotFound", err)
	}
}
