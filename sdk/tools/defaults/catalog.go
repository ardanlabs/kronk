package defaults

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed yaml/catalog.yaml
var catalogFS embed.FS

const (
	// catalogDirName is the per-user catalog directory under BaseDir.
	// It owns catalog.yaml.
	catalogDirName = "catalog"

	// catalogFileName is the on-disk YAML filename inside catalogDirName.
	catalogFileName = "catalog.yaml"

	// embeddedCatalogPath is the path inside catalogFS, which mirrors
	// the //go:embed directive above.
	embeddedCatalogPath = "yaml/catalog.yaml"
)

// CatalogFile returns the path to the catalog.yaml file. If no override is
// provided, it ensures ~/.kronk/catalog/ exists and seeds the embedded
// default to ~/.kronk/catalog/catalog.yaml when no file is present.
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

	data, err := catalogFS.ReadFile(embeddedCatalogPath)
	if err != nil {
		return "", fmt.Errorf("catalog-file: reading embedded file: %w", err)
	}

	if err := os.WriteFile(target, data, 0644); err != nil {
		return "", fmt.Errorf("catalog-file: writing default file: %w", err)
	}

	return target, nil
}
