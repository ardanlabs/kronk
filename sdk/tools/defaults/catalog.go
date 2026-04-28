package defaults

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed yaml/catalog.yaml
var catalogFS embed.FS

const (
	// catalogDirName is the per-user catalog directory under BaseDir.
	// It owns catalog.yaml and the gguf_cache/ subdirectory.
	catalogDirName = "catalog"

	// catalogFileName is the on-disk YAML filename inside catalogDirName.
	catalogFileName = "catalog.yaml"

	// embeddedCatalogPath is the path inside catalogFS, which mirrors
	// the //go:embed directive above.
	embeddedCatalogPath = "yaml/catalog.yaml"

	// ggufCacheDirName is the subdirectory of catalogDirName that holds
	// raw GGUF head bytes per <provider>/<family>/<modelID>.gguf.
	ggufCacheDirName = "gguf_cache"
)

// CatalogFile returns the path to the catalog.yaml file. If no override is
// provided, it ensures ~/.kronk/catalog/ exists and seeds the embedded
// default to ~/.kronk/catalog/catalog.yaml when no file is present. If an
// older ~/.kronk/catalog.yaml exists at the legacy location and no new file
// exists, the legacy file is moved to the new location.
func CatalogFile(override string, basePath string) (string, error) {
	if override != "" {
		return override, nil
	}

	basePath = BaseDir(basePath)

	catalogDir := filepath.Join(basePath, catalogDirName)
	target := filepath.Join(catalogDir, catalogFileName)

	if _, err := os.Stat(target); err == nil {
		return target, nil
	}

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return "", fmt.Errorf("catalog-file: creating catalog directory: %w", err)
	}

	// Migrate the legacy ~/.kronk/catalog.yaml if it exists.
	legacy := filepath.Join(basePath, catalogFileName)
	if _, err := os.Stat(legacy); err == nil {
		if err := os.Rename(legacy, target); err != nil {
			return "", fmt.Errorf("catalog-file: migrating legacy file: %w", err)
		}
		return target, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("catalog-file: stat legacy: %w", err)
	}

	data, err := catalogFS.ReadFile(embeddedCatalogPath)
	if err != nil {
		return "", fmt.Errorf("catalog-file: reading embedded file: %w", err)
	}

	if err := os.WriteFile(target, data, 0644); err != nil {
		return "", fmt.Errorf("catalog-file: writing default file: %w", err)
	}

	return target, nil
}

// GGUFCacheDir returns ~/.kronk/catalog/gguf_cache/, creating it (and the
// parent catalog directory) on demand.
func GGUFCacheDir(basePath string) (string, error) {
	basePath = BaseDir(basePath)
	dir := filepath.Join(basePath, catalogDirName, ggufCacheDirName)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("gguf-cache-dir: %w", err)
	}

	return dir, nil
}

// EmbeddedCatalog returns the embedded default catalog.yaml content. Useful
// for callers that need the seed bytes without writing them to disk.
func EmbeddedCatalog() ([]byte, error) {
	data, err := catalogFS.ReadFile(embeddedCatalogPath)
	if err != nil {
		return nil, fmt.Errorf("embedded-catalog: %w", err)
	}

	return data, nil
}
