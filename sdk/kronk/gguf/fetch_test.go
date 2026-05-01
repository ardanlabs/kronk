package gguf

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchRangeEOFClamped(t *testing.T) {
	const fileSize = 7872576

	body := make([]byte, fileSize)
	for i := range body {
		body[i] = byte(i % 256)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", fileSize-1, fileSize))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body)
	}))
	defer ts.Close()

	client := http.Client{}
	data, fs, err := fetchRangeWithClient(context.Background(), &client, ts.URL, 0, 16*1024*1024-1)
	if err != nil {
		t.Fatalf("expected success for EOF-clamped 206, got error: %v", err)
	}
	if fs != int64(fileSize) {
		t.Errorf("fileSize = %d, want %d", fs, fileSize)
	}
	if len(data) != fileSize {
		t.Errorf("len(data) = %d, want %d", len(data), fileSize)
	}
}

func TestFetchRangeShortReadStillFails(t *testing.T) {
	const fileSize = 7872576

	body := make([]byte, fileSize-1000) // genuinely truncated

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", fileSize-1, fileSize))
		w.WriteHeader(http.StatusPartialContent)
		w.Write(body)
	}))
	defer ts.Close()

	client := http.Client{}
	_, _, err := fetchRangeWithClient(context.Background(), &client, ts.URL, 0, 16*1024*1024-1)
	if err == nil {
		t.Fatal("expected short-read error for truncated body, got nil")
	}
}
