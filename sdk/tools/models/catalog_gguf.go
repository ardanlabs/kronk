package models

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// GGUFHead returns the first ggufHeaderFetchSize bytes of the catalog
// entry's primary model file. Lookup order:
//
//  1. The catalog GGUF cache at
//     <basePath>/catalog/gguf_cache/<provider>/<family>/<modelID>.gguf.
//  2. The local model file under <modelsPath>/<provider>/<family>/<file>
//     when the model is already downloaded.
//  3. An HTTP Range request to HuggingFace.
//
// On cache miss the bytes are written through to the catalog GGUF cache
// so the next call is a fast cache hit. Callers parse the bytes using
// the GGUF binary format (see vram.go's FetchGGUFMetadata).
func (m *Models) GGUFHead(ctx context.Context, entry CatalogEntry) ([]byte, error) {
	if len(entry.Files) == 0 {
		return nil, fmt.Errorf("gguf-head: catalog entry has no files")
	}

	modelID := extractModelID(entry.Files[0])

	cacheFile, err := ggufCacheFile(m.basePath, entry.Provider, entry.Family, modelID)
	if err != nil {
		return nil, fmt.Errorf("gguf-head: cache path: %w", err)
	}

	if data, ok := readCachedGGUFHead(cacheFile); ok {
		return data, nil
	}

	// Try the local file before the network.
	localFile := filepath.Join(m.modelsPath, entry.Provider, entry.Family, filepath.Base(entry.Files[0]))
	if data, ok := readLocalGGUFHead(localFile); ok {
		writeCachedGGUFHead(cacheFile, data)
		return data, nil
	}

	if !hasNetwork() {
		return nil, fmt.Errorf("gguf-head: no cache, no local file, no network for %s/%s", entry.Provider, entry.Family)
	}

	url := buildDownloadURL(entry.Provider, entry.Family, entry.Revision, entry.Files[0])

	data, _, err := fetchGGUFHeaderBytes(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("gguf-head: fetch %s: %w", url, err)
	}

	writeCachedGGUFHead(cacheFile, data)

	return data, nil
}

// CacheGGUFHeadFromFile copies the first ggufHeaderFetchSize bytes of
// localFile into the catalog GGUF cache for the given canonical id.
// Used by Download to populate the cache opportunistically after a
// successful download. Best-effort — errors are returned but callers
// typically log and continue.
func (m *Models) CacheGGUFHeadFromFile(provider, family, modelID, localFile string) error {
	cacheFile, err := ggufCacheFile(m.basePath, provider, family, modelID)
	if err != nil {
		return fmt.Errorf("cache-gguf-head: %w", err)
	}

	data, ok := readLocalGGUFHead(localFile)
	if !ok {
		return fmt.Errorf("cache-gguf-head: read %s", localFile)
	}

	if err := writeCachedGGUFHead(cacheFile, data); err != nil {
		return fmt.Errorf("cache-gguf-head: write %s: %w", cacheFile, err)
	}

	return nil
}

// RemoveGGUFHeadCache deletes the catalog GGUF cache file for the given
// canonical id. Called when an entry is removed from the catalog.
func (m *Models) RemoveGGUFHeadCache(provider, family, modelID string) error {
	cacheFile, err := ggufCacheFile(m.basePath, provider, family, modelID)
	if err != nil {
		return fmt.Errorf("remove-gguf-head-cache: %w", err)
	}

	if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove-gguf-head-cache: %w", err)
	}

	// Best-effort cleanup of empty parent directories.
	familyDir := filepath.Dir(cacheFile)
	os.Remove(familyDir)
	os.Remove(filepath.Dir(familyDir))

	return nil
}

// =============================================================================

// ggufCacheFile returns the absolute path to the cache file for a single
// catalog entry. Creates parent directories on demand.
func ggufCacheFile(basePath, provider, family, modelID string) (string, error) {
	dir, err := defaults.GGUFCacheDir(basePath)
	if err != nil {
		return "", err
	}

	familyDir := filepath.Join(dir, provider, family)
	if err := os.MkdirAll(familyDir, 0755); err != nil {
		return "", fmt.Errorf("gguf-cache-file: mkdir: %w", err)
	}

	return filepath.Join(familyDir, modelID+".gguf"), nil
}

// readCachedGGUFHead returns cached GGUF head bytes when the cache file
// exists and parses as a valid GGUF header.
func readCachedGGUFHead(path string) ([]byte, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}

	if !isValidGGUFHeaderBytes(data) {
		return nil, false
	}

	return data, true
}

// readLocalGGUFHead reads the first ggufHeaderFetchSize bytes of a local
// GGUF file. Returns false when the file is missing or the read fails.
func readLocalGGUFHead(path string) ([]byte, bool) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, ggufHeaderFetchSize))
	if err != nil {
		return nil, false
	}

	if !isValidGGUFHeaderBytes(data) {
		return nil, false
	}

	return data, true
}

// writeCachedGGUFHead writes head bytes atomically to the cache file.
// Best-effort — caller should not block on errors.
func writeCachedGGUFHead(path string, data []byte) error {
	if !isValidGGUFHeaderBytes(data) {
		return fmt.Errorf("write-cached-gguf-head: invalid header bytes")
	}

	return writeFileAtomic(path, data, 0644)
}
