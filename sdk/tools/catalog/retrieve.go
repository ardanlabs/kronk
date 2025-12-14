package catalog

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools"
	"go.yaml.in/yaml/v2"
)

// CatalogModelList returns the collection of models in the catalog with
// some filtering capabilities.
func CatalogModelList(basePath string, filterCategory string) ([]Model, error) {
	catalogs, err := RetrieveCatalogs(basePath)
	if err != nil {
		return nil, fmt.Errorf("catalog list: %w", err)
	}

	modelBasePath := defaults.ModelsDir("")

	modelFiles, err := tools.RetrieveModelFiles(modelBasePath)
	if err != nil {
		return nil, fmt.Errorf("retrieve-model-files: %w", err)
	}

	pulledModels := make(map[string]struct{})
	for _, mf := range modelFiles {
		pulledModels[strings.ToLower(mf.ID)] = struct{}{}
	}

	filterLower := strings.ToLower(filterCategory)

	var list []Model
	for _, cat := range catalogs {
		if filterCategory != "" && !strings.Contains(strings.ToLower(cat.Name), filterLower) {
			continue
		}

		for _, model := range cat.Models {
			_, downloaded := pulledModels[strings.ToLower(model.ID)]
			model.Downloaded = downloaded
			list = append(list, model)
		}
	}

	slices.SortFunc(list, func(a, b Model) int {
		if c := cmp.Compare(strings.ToLower(a.Category), strings.ToLower(b.Category)); c != 0 {
			return c
		}
		return cmp.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return list, nil
}

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
	catalogDir := filepath.Join(basePath, localFolder)

	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		return nil, fmt.Errorf("read catalog dir: %w", err)
	}

	var catalogs []Catalog

	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == indexFile {
			continue
		}

		catalog, err := RetrieveCatalog(basePath, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("retrieve-catalog: %q: %w", entry.Name(), err)
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
