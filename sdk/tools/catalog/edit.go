package catalog

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v2"
)

// CatalogFileInfo represents a catalog file on disk.
type CatalogFileInfo struct {
	Name       string `json:"name"`
	ModelCount int    `json:"model_count"`
}

// ListCatalogFiles returns the catalog file names available on disk.
func (c *Catalog) ListCatalogFiles() ([]CatalogFileInfo, error) {
	names, err := c.catalogYAMLFiles()
	if err != nil {
		return nil, fmt.Errorf("list-catalog-files: %w", err)
	}

	var files []CatalogFileInfo
	for _, name := range names {
		cat, err := c.singleCatalog(name)
		if err != nil {
			continue
		}

		files = append(files, CatalogFileInfo{
			Name:       name,
			ModelCount: len(cat.Models),
		})
	}

	return files, nil
}

// SaveModel adds or updates a model entry in the specified catalog file.
// If the catalog file does not exist, it is created. If a model with the
// same ID already exists in the file, it is replaced.
func (c *Catalog) SaveModel(model ModelDetails, catalogFile string) error {
	if model.ID == "" {
		return fmt.Errorf("save-model: model ID is required")
	}

	if catalogFile == "" {
		return fmt.Errorf("save-model: catalog file name is required")
	}

	if !strings.HasSuffix(catalogFile, ".yaml") {
		catalogFile += ".yaml"
	}

	filePath := filepath.Join(c.catalogPath, catalogFile)

	var cat CatalogModels

	data, err := os.ReadFile(filePath)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &cat); err != nil {
			return fmt.Errorf("save-model: unmarshal existing catalog %s: %w", catalogFile, err)
		}
	default:
		cat.Name = model.Category
	}

	replaced := false
	for i, m := range cat.Models {
		if strings.EqualFold(m.ID, model.ID) {
			cat.Models[i] = model
			replaced = true
			break
		}
	}

	if !replaced {
		cat.Models = append(cat.Models, model)
	}

	out, err := marshalCatalog(&cat)
	if err != nil {
		return fmt.Errorf("save-model: marshal catalog: %w", err)
	}

	if err := os.WriteFile(filePath, out, 0644); err != nil {
		return fmt.Errorf("save-model: write catalog file: %w", err)
	}

	if err := c.buildIndex(); err != nil {
		return fmt.Errorf("save-model: rebuild index: %w", err)
	}

	return nil
}

// RepoPath returns the configured repository path. An empty string means
// publishing is not configured.
func (c *Catalog) RepoPath() string {
	return c.repoPath
}

// PublishModel copies the specified catalog file from the local catalog
// directory to the cloned repository path.
func (c *Catalog) PublishModel(catalogFile string) error {
	if c.repoPath == "" {
		return fmt.Errorf("publish-model: catalog repo path is not configured")
	}

	if catalogFile == "" {
		return fmt.Errorf("publish-model: catalog file name is required")
	}

	if !strings.HasSuffix(catalogFile, ".yaml") {
		catalogFile += ".yaml"
	}

	srcPath := filepath.Join(c.catalogPath, catalogFile)

	data, err := os.ReadFile(srcPath)
	if err != nil {
		return fmt.Errorf("publish-model: read source catalog %s: %w", catalogFile, err)
	}

	dstDir := filepath.Join(c.repoPath, "catalogs")
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("publish-model: create repo catalogs directory: %w", err)
	}

	dstPath := filepath.Join(dstDir, catalogFile)
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("publish-model: write to repo %s: %w", dstPath, err)
	}

	return nil
}

// DeleteModel removes a model entry from the catalog by its ID.
func (c *Catalog) DeleteModel(modelID string) error {
	if modelID == "" {
		return fmt.Errorf("delete-model: model ID is required")
	}

	index, err := c.loadIndex()
	if err != nil {
		return fmt.Errorf("delete-model: load index: %w", err)
	}

	catalogFile, ok := index[modelID]
	if !ok {
		return fmt.Errorf("delete-model: model %q not found in index", modelID)
	}

	cat, err := c.singleCatalog(catalogFile)
	if err != nil {
		return fmt.Errorf("delete-model: read catalog: %w", err)
	}

	found := false
	models := make([]ModelDetails, 0, len(cat.Models))
	for _, m := range cat.Models {
		if strings.EqualFold(m.ID, modelID) {
			found = true
			continue
		}
		models = append(models, m)
	}

	if !found {
		return fmt.Errorf("delete-model: model %q not found in catalog %s", modelID, catalogFile)
	}

	cat.Models = models

	out, err := marshalCatalog(&cat)
	if err != nil {
		return fmt.Errorf("delete-model: marshal catalog: %w", err)
	}

	filePath := filepath.Join(c.catalogPath, catalogFile)
	if err := os.WriteFile(filePath, out, 0644); err != nil {
		return fmt.Errorf("delete-model: write catalog file: %w", err)
	}

	if err := c.buildIndex(); err != nil {
		return fmt.Errorf("delete-model: rebuild index: %w", err)
	}

	return nil
}

// marshalCatalog marshals a CatalogModels value to YAML and inserts a blank
// line between each model entry for readability.
func marshalCatalog(cat *CatalogModels) ([]byte, error) {
	out, err := yaml.Marshal(cat)
	if err != nil {
		return nil, err
	}

	out = bytes.ReplaceAll(out, []byte("\n- id:"), []byte("\n\n- id:"))

	return out, nil
}
