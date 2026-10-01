package hf

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingReader struct {
	read bool
}

func (fr *failingReader) Read(p []byte) (int, error) {
	if fr.read {
		return 0, errors.New("connection reset")
	}

	fr.read = true
	return copy(p, "partial"), nil
}

func TestDefaultClientDoReturnsBodyReadError(t *testing.T) {
	c := DefaultClient{
		HTTP: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(&failingReader{}),
				}, nil
			}),
		},
	}

	_, err := c.do(context.Background(), "https://huggingface.co/api/models/owner/repo")
	if err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("do: got %v, want connection reset error", err)
	}
}
