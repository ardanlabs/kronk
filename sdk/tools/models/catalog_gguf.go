package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

// GGUFHead returns the first gguf.HeaderFetchSize bytes of the catalog
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
// the GGUF binary format (see sdk/kronk/gguf.ParseMetadata).
func (m *Models) GGUFHead(ctx context.Context, entry CatalogEntry) ([]byte, error) {
	if len(entry.Files) == 0 {
		return nil, fmt.Errorf("gguf-head: catalog entry has no files")
	}

	modelID := extractModelID(entry.Files[0])

	cacheFile, err := ggufCacheFile(m.basePath, entry.Provider, entry.Family, modelID)
	if err != nil {
		return nil, fmt.Errorf("gguf-head: cache path: %w", err)
	}

	if data, err := gguf.ReadHeaderBytes(cacheFile); err == nil && gguf.IsValidHeaderBytes(data) {
		return data, nil
	}

	// Try the local file before the network.
	localFile := filepath.Join(m.modelsPath, entry.Provider, entry.Family, filepath.Base(entry.Files[0]))
	if data, err := gguf.ReadHeaderBytes(localFile); err == nil && gguf.IsValidHeaderBytes(data) {
		_ = gguf.WriteHeaderBytes(cacheFile, data)
		return data, nil
	}

	if !hasNetwork() {
		return nil, fmt.Errorf("gguf-head: no cache, no local file, no network for %s/%s", entry.Provider, entry.Family)
	}

	url := buildDownloadURL(entry.Provider, entry.Family, entry.Revision, entry.Files[0])

	data, _, err := gguf.FetchHeaderBytes(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("gguf-head: fetch %s: %w", url, err)
	}

	_ = gguf.WriteHeaderBytes(cacheFile, data)

	return data, nil
}

// CacheGGUFHeadFromFile copies the first gguf.HeaderFetchSize bytes of
// localFile into the catalog GGUF cache for the given canonical id.
// Used by Download to populate the cache opportunistically after a
// successful download. Best-effort — errors are returned but callers
// typically log and continue.
func (m *Models) CacheGGUFHeadFromFile(provider, family, modelID, localFile string) error {
	cacheFile, err := ggufCacheFile(m.basePath, provider, family, modelID)
	if err != nil {
		return fmt.Errorf("cache-gguf-head: %w", err)
	}

	data, err := gguf.ReadHeaderBytes(localFile)
	if err != nil || !gguf.IsValidHeaderBytes(data) {
		return fmt.Errorf("cache-gguf-head: read %s", localFile)
	}

	if err := gguf.WriteHeaderBytes(cacheFile, data); err != nil {
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
// catalog entry. Creates parent directories on demand. The cache lives at
// <basePath>/catalog/gguf_cache/<provider>/<family>/<modelID>.gguf.
func ggufCacheFile(basePath, provider, family, modelID string) (string, error) {
	familyDir := filepath.Join(defaults.BaseDir(basePath), "catalog", "gguf_cache", provider, family)
	if err := os.MkdirAll(familyDir, 0755); err != nil {
		return "", fmt.Errorf("gguf-cache-file: mkdir: %w", err)
	}

	return filepath.Join(familyDir, modelID+".gguf"), nil
}
