package gguf

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// FetchHeaderBytes fetches the first HeaderFetchSize bytes of the GGUF
// file at rawURL via an HTTP Range request and returns the bytes plus
// the total file size advertised by the server.
//
// Header bytes are not cached on disk by this function. Callers that
// want a persistent cache (e.g. the catalog gguf_cache) are expected to
// layer their own storage above this call.
func FetchHeaderBytes(ctx context.Context, rawURL string) ([]byte, int64, error) {
	data, fileSize, err := FetchRange(ctx, rawURL, 0, HeaderFetchSize-1)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-header-bytes: failed to fetch header data: %w", err)
	}

	return data, fileSize, nil
}

// ReadHeaderBytes reads up to the first HeaderFetchSize bytes of a
// local GGUF file. No magic validation is performed; callers that want
// it should pair the result with IsValidHeaderBytes. Smaller files
// (e.g. test fixtures or a fully-cached header that happens to equal
// HeaderFetchSize) are read in full.
func ReadHeaderBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read-gguf-header-bytes: open: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, HeaderFetchSize))
	if err != nil {
		return nil, fmt.Errorf("read-gguf-header-bytes: read: %w", err)
	}

	return data, nil
}

// WriteHeaderBytes validates the GGUF magic on data and writes it to
// path atomically (temp file + rename so partial writes never replace
// a valid file). Returns an error if validation or any filesystem
// operation fails.
func WriteHeaderBytes(path string, data []byte) error {
	if !IsValidHeaderBytes(data) {
		return fmt.Errorf("write-gguf-header-bytes: invalid header bytes")
	}

	return writeFileAtomic(path, data, 0644)
}

// writeFileAtomic writes data to path via a temp file + rename so
// partial writes never replace a valid file.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)

	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write-file-atomic: create temp file: %w", err)
	}
	tmpPath := f.Name()

	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("write-file-atomic: write temp file: %w", err)
	}

	if err := f.Chmod(perm); err != nil {
		_ = f.Close()
		return fmt.Errorf("write-file-atomic: chmod temp file: %w", err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("write-file-atomic: close temp file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("write-file-atomic: rename temp file: %w", err)
	}

	return nil
}

// IsValidHeaderBytes reports whether data starts with the GGUF magic
// number. Used as a sanity check before persisting bytes to a cache or
// trusting them from disk.
func IsValidHeaderBytes(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	return binary.LittleEndian.Uint32(data[:4]) == Magic
}

// FetchRange fetches a byte range from a URL using HTTP Range requests.
// Returns the requested bytes and the total file size advertised by the
// server (when known).
//
// When KRONK_HF_TOKEN is set the request carries it as a Bearer
// Authorization header so gated HuggingFace repos resolve. When the
// server returns 200 OK (full file) instead of 206 Partial Content —
// which happens for some HuggingFace storage backends like Xet that do
// not honor Range — the function reads only the requested range from
// the response body to avoid downloading the whole file.
func FetchRange(ctx context.Context, url string, start, end int64) ([]byte, int64, error) {
	var client http.Client
	return fetchRangeWithClient(ctx, &client, url, start, end)
}

// fetchRangeWithClient is the internal Range fetcher that lets tests
// inject their own *http.Client.
func fetchRangeWithClient(ctx context.Context, client *http.Client, url string, start, end int64) ([]byte, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	if token := os.Getenv("KRONK_HF_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("fetch-range: unexpected status code: %d, url=%s", resp.StatusCode, resp.Request.URL.Host)
	}

	cr := resp.Header.Get("Content-Range")

	var (
		fileSize  int64
		respStart int64
		respEnd   int64
		haveRange bool
	)

	if cr != "" {
		if n, _ := fmt.Sscanf(cr, "bytes %d-%d/%d", &respStart, &respEnd, &fileSize); n == 3 {
			haveRange = true
		}
	} else if resp.ContentLength > 0 && resp.StatusCode == http.StatusOK {
		fileSize = resp.ContentLength
	}

	// When the server returns 200 OK (full file) instead of 206 Partial
	// Content, read only the requested range to avoid downloading the
	// entire file. This happens when HuggingFace redirects to a storage
	// backend (e.g., Xet) that does not support HTTP Range requests.
	var reader io.Reader = resp.Body
	if resp.StatusCode == http.StatusOK {
		if start > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, start); err != nil {
				return nil, 0, fmt.Errorf("fetch-range: failed to skip to offset %d: %w", start, err)
			}
		}
		reader = io.LimitReader(resp.Body, end-start+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-range: read body failed: status=%d, requested_range=%d-%d, content_range=%q, content_length=%d, host=%s: %w",
			resp.StatusCode, start, end, cr, resp.ContentLength, resp.Request.URL.Host, err)
	}

	if resp.StatusCode == http.StatusPartialContent {
		switch {
		case haveRange:
			// When the requested range extends past EOF the server returns the
			// satisfiable subrange. Clamp our expectation to match.
			expectedEnd := end
			if fileSize > 0 && expectedEnd >= fileSize {
				expectedEnd = fileSize - 1
			}

			if respStart != start || respEnd != expectedEnd {
				return nil, 0, fmt.Errorf("fetch-range: unexpected content-range: requested=%d-%d, got=%q, host=%s",
					start, end, cr, resp.Request.URL.Host)
			}

			expectedLen := respEnd - respStart + 1
			if int64(len(data)) != expectedLen {
				return nil, 0, fmt.Errorf("fetch-range: short read: got %d bytes, expected %d, status=%d, content_range=%q, host=%s",
					len(data), expectedLen, resp.StatusCode, cr, resp.Request.URL.Host)
			}

		default:
			// No parseable Content-Range; fall back to the original check.
			if int64(len(data)) < end-start+1 {
				return nil, 0, fmt.Errorf("fetch-range: short read: got %d bytes, expected %d, status=%d, content_range=%q, host=%s",
					len(data), end-start+1, resp.StatusCode, cr, resp.Request.URL.Host)
			}
		}
	}

	return data, fileSize, nil
}
