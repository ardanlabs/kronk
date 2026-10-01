package gguf

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestFetchRangeAuthorizesHuggingFace(t *testing.T) {
	t.Setenv("KRONK_HF_TOKEN", "secret")

	client := http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "Bearer secret" {
				t.Errorf("Authorization: got %q, want %q", got, "Bearer secret")
			}

			return &http.Response{
				StatusCode:    http.StatusPartialContent,
				Header:        http.Header{"Content-Range": []string{"bytes 0-0/1"}},
				Body:          io.NopCloser(strings.NewReader("x")),
				ContentLength: 1,
				Request:       r,
			}, nil
		}),
	}

	data, size, err := fetchRangeWithClient(t.Context(), &client, "https://huggingface.co/owner/repo/resolve/main/model.gguf", 0, 0)
	if err != nil {
		t.Fatalf("fetchRangeWithClient: %v", err)
	}
	if string(data) != "x" || size != 1 {
		t.Errorf("range result: got data %q and size %d, want data %q and size 1", data, size, "x")
	}
}

func TestFetchRangeEOFClamped(t *testing.T) {
	const fileSize = 7872576

	body := make([]byte, fileSize)
	for i := range body {
		body[i] = byte(i % 256)
	}

	t.Setenv("KRONK_HF_TOKEN", "secret")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization: got %q, want empty for non-Hugging Face host", got)
		}
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
