package models

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsHuggingFaceFolderURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{
			name: "shorthand-with-tag",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "shorthand-with-revision",
			url:  "bartowski/Qwen3-8B-GGUF:Q4_K_M@main",
			want: false,
		},
		{
			name: "shorthand-with-hf-prefix",
			url:  "hf.co/bartowski/Qwen3-8B-GGUF:Q4_K_M",
			want: false,
		},
		{
			name: "full-gguf-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "short-form-gguf",
			url:  "Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "blob-url",
			url:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/blob/main/Qwen3-8B-Q8_0.gguf",
			want: false,
		},
		{
			name: "folder-tree-url",
			url:  "https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF/tree/main/UD-Q5_K_XL",
			want: true,
		},
		{
			name: "short-form-folder",
			url:  "owner/repo/subfolder",
			want: true,
		},
		{
			name: "owner-repo-only",
			url:  "owner/repo",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHuggingFaceFolderURL(tt.url)
			if got != tt.want {
				t.Errorf("isHuggingFaceFolderURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

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
	data, fs, err := fetchRange(context.Background(), &client, ts.URL, 0, 16*1024*1024-1)
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
	_, _, err := fetchRange(context.Background(), &client, ts.URL, 0, 16*1024*1024-1)
	if err == nil {
		t.Fatal("expected short-read error for truncated body, got nil")
	}
}

func TestNormalizeHuggingFaceDownloadURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "blob-to-resolve",
			in:   "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/blob/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
		{
			name: "resolve-unchanged",
			in:   "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
		{
			name: "shorthand",
			in:   "unsloth/Qwen3.5-35B-A3B-GGUF/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
			want: "https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-MXFP4_MOE.gguf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeHuggingFaceDownloadURL(tt.in)
			if got != tt.want {
				t.Errorf("NormalizeHuggingFaceDownloadURL(%q)\n got  %q\n want %q", tt.in, got, tt.want)
			}
		})
	}
}
