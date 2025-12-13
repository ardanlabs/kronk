package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// RetrieveModelDetails returns the full model information for the
// specified model.
func RetrieveModelDetails(basePath string, modelID string) (Model, error) {
	index, err := loadIndex(basePath)
	if err != nil {
		return Model{}, fmt.Errorf("load-index: %w", err)
	}

	modelID = strings.ToLower(modelID)

	catalogFile := index[modelID]
	if catalogFile == "" {
		return Model{}, fmt.Errorf("model %q not found in index", modelID)
	}

	catalog, err := RetrieveCatalog(basePath, catalogFile)
	if err != nil {
		return Model{}, fmt.Errorf("retrieve-catalog: %w", err)
	}

	for _, model := range catalog.Models {
		id := strings.ToLower(model.ID)
		if strings.EqualFold(id, modelID) {
			return model, nil
		}
	}

	return Model{}, fmt.Errorf("model %q not found", modelID)
}

// RetrieveCatalog returns an individual catalog by the base catalog file name.
func RetrieveCatalog(basePath string, catalogFile string) (Catalog, error) {
	filePath := filepath.Join(basePath, localFolder, catalogFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return Catalog{}, fmt.Errorf("read file %s: %w", catalogFile, err)
	}

	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return Catalog{}, fmt.Errorf("unmarshal %s: %w", catalogFile, err)
	}

	return catalog, nil
}

// RetrieveCatalogs reads the catalogs from a previous download.
func RetrieveCatalogs(basePath string) ([]Catalog, error) {
	index, err := loadIndex(basePath)
	if err != nil {
		return nil, fmt.Errorf("load-index: %w", err)
	}

	var catalogs []Catalog

	for _, catalogFile := range index {
		catalog, err := RetrieveCatalog(basePath, catalogFile)
		if err != nil {
			return nil, fmt.Errorf("retrieve-catalog: %q: %w", catalogFile, err)
		}

		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil
}

// =============================================================================

// LoadIndex returns the catalog index.
func loadIndex(modelBasePath string) (map[string]string, error) {
	indexPath := filepath.Join(modelBasePath, localFolder, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if err := buildIndex(modelBasePath); err != nil {
			return nil, fmt.Errorf("build-index: %w", err)
		}
		data, err = os.ReadFile(indexPath)
		if err != nil {
			return nil, fmt.Errorf("read-index: %w", err)
		}
	}

	var index map[string]string
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("unmarshal-index: %w", err)
	}

	return index, nil
}
