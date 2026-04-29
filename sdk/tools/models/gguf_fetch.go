package models

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// fetchGGUFHeaderBytes fetches the first ggufHeaderFetchSize bytes of the
// GGUF file at rawURL via an HTTP Range request and returns the bytes plus
// the total file size advertised by the server.
//
// Header bytes are not cached on disk. The two persistent GGUF caches are
// the catalog gguf_cache (canonical-id-keyed, populated on download and on
// first detail-panel view) and the on-disk model file itself; both are
// preferred over this network call by GGUFHead.
func fetchGGUFHeaderBytes(ctx context.Context, rawURL string) ([]byte, int64, error) {
	var client http.Client

	data, fileSize, err := fetchRange(ctx, &client, rawURL, 0, ggufHeaderFetchSize-1)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-header-bytes: failed to fetch header data: %w", err)
	}

	return data, fileSize, nil
}

// isValidGGUFHeaderBytes reports whether data starts with the GGUF magic
// number. Used as a sanity check before persisting bytes to the catalog
// GGUF cache or trusting them from disk.
func isValidGGUFHeaderBytes(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	return binary.LittleEndian.Uint32(data[:4]) == ggufMagic
}

// writeFileAtomic writes data to path via a temp file + rename so partial
// writes never replace a valid file.
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
