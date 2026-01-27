package catalog

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"go.yaml.in/yaml/v2"
)

// CatalogModelList returns the collection of models in the catalog with
// some filtering capabilities.
func (c *Catalog) CatalogModelList(filterCategory string) ([]Model, error) {
	catalogs, err := c.RetrieveCatalogs()
	if err != nil {
		return nil, fmt.Errorf("catalog-model-list: catalog list: %w", err)
	}

	modelFiles, err := c.models.RetrieveFiles()
	if err != nil {
		return nil, fmt.Errorf("catalog-model-list: retrieve-model-files: %w", err)
	}

	pulledModels := make(map[string]struct{})
	validatedModels := make(map[string]struct{})

	for _, mf := range modelFiles {
		pulledModels[mf.ID] = struct{}{}
		if mf.Validated {
			validatedModels[mf.ID] = struct{}{}
		}
	}

	var list []Model
	for _, cat := range catalogs {
		if filterCategory != "" && !strings.Contains(strings.ToLower(cat.Name), strings.ToLower(filterCategory)) {
			continue
		}

		for _, model := range cat.Models {
			_, downloaded := pulledModels[model.ID]
			model.Downloaded = downloaded

			_, validated := validatedModels[model.ID]
			model.Validated = validated

			list = append(list, model)
		}
	}

	slices.SortFunc(list, func(a, b Model) int {
		if c := cmp.Compare(a.Category, b.Category); c != 0 {
			return c
		}

		// Using ToLower to help with case-insensitve sorting.
		return cmp.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return list, nil
}

// RetrieveModelDetails returns the full model information for the
// specified model.
func (c *Catalog) RetrieveModelDetails(modelID string) (Model, error) {
	index, err := c.loadIndex()
	if err != nil {
		return Model{}, fmt.Errorf("retrieve-model-details: load-index: %w", err)
	}

	catalogFile := index[modelID]
	if catalogFile == "" {
		return Model{}, fmt.Errorf("retrieve-model-details: model[%s] not found in index", modelID)
	}

	catalog, err := c.RetrieveCatalog(catalogFile)
	if err != nil {
		return Model{}, fmt.Errorf("retrieve-model-details: retrieve-catalog: %w", err)
	}

	for _, model := range catalog.Models {
		if strings.EqualFold(model.ID, modelID) {
			return model, nil
		}
	}

	return Model{}, fmt.Errorf("retrieve-model-details: model[%s] not found", modelID)
}

// RetrieveCatalog returns an individual catalog by the base catalog file name.
func (c *Catalog) RetrieveCatalog(catalogFile string) (CatalogModels, error) {
	filePath := filepath.Join(c.catalogPath, catalogFile)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return CatalogModels{}, fmt.Errorf("retrieve-catalog: read file catalog-file[%s]: %w", catalogFile, err)
	}

	var catalog CatalogModels
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return CatalogModels{}, fmt.Errorf("retrieve-catalog: unmarshal catalog-file[%s]: %w", catalogFile, err)
	}

	return catalog, nil
}

// RetrieveCatalogs reads the catalogs from a previous download.
func (c *Catalog) RetrieveCatalogs() ([]CatalogModels, error) {
	entries, err := os.ReadDir(c.catalogPath)
	if err != nil {
		return nil, fmt.Errorf("retrieve-catalogs: read catalog dir: %w", err)
	}

	var catalogs []CatalogModels

	for _, entry := range entries {
		if entry.IsDir() ||
			entry.Name() == indexFile ||
			entry.Name() == shaFile ||
			entry.Name() == ".DS_Store" {
			continue
		}

		catalog, err := c.RetrieveCatalog(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("retrieve-catalogs: retrieve-catalog name[%s]: %w", entry.Name(), err)
		}

		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil
}

// RetrieveModelConfig reads the catalog and model config file for the
// specified model id and returns a model config for use.
func (c *Catalog) RetrieveModelConfig(modelID string) (model.Config, error) {

	// The modelID might have a / because it's a model in the config
	// with different settings. Ex. model/FMC.
	// We need to remove /FMC for the RetrievePath call.
	fsModelID, _, _ := strings.Cut(modelID, "/")

	// Get the file path for this model on disk. If this fails, the
	// model hasn't been downloaded and nothing else to do.
	fp, err := c.models.RetrievePath(fsModelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("retrieve-model-config: unable to get model[%s] path: %w", modelID, err)
	}

	// Look in the catalog config first for the specified model.
	var catalogFound bool
	catalog, err := c.RetrieveModelDetails(modelID)
	if err == nil {
		catalogFound = true
	}

	// Look in the model config for the specified model.
	modelConfig, modelCfgFound := c.modelConfig[modelID]

	// Set the file paths.
	cfg := model.Config{
		ModelFiles: fp.ModelFiles,
		ProjFile:   fp.ProjFile,
	}

	// Apply catalog settings first if found.
	if catalogFound {
		c := catalog.ModelConfig
		cfg.Device = c.Device
		cfg.ContextWindow = c.ContextWindow
		cfg.NBatch = c.NBatch
		cfg.NUBatch = c.NUBatch
		cfg.NThreads = c.NThreads
		cfg.NThreadsBatch = c.NThreadsBatch
		cfg.CacheTypeK = c.CacheTypeK
		cfg.CacheTypeV = c.CacheTypeV
		cfg.FlashAttention = c.FlashAttention
		cfg.UseDirectIO = c.UseDirectIO
		cfg.IgnoreIntegrityCheck = c.IgnoreIntegrityCheck
		cfg.NSeqMax = c.NSeqMax
		cfg.OffloadKQV = c.OffloadKQV
		cfg.OpOffload = c.OpOffload
		cfg.NGpuLayers = c.NGpuLayers
		cfg.SplitMode = c.SplitMode
		cfg.SystemPromptCache = c.SystemPromptCache
		cfg.FirstMessageCache = c.FirstMessageCache
		cfg.CacheMinTokens = c.CacheMinTokens
	}

	// Apply model config settings if found (overrides catalog).
	if modelCfgFound {
		if modelConfig.Device != "" {
			cfg.Device = modelConfig.Device
		}
		if modelConfig.ContextWindow != 0 {
			cfg.ContextWindow = modelConfig.ContextWindow
		}
		if modelConfig.NBatch != 0 {
			cfg.NBatch = modelConfig.NBatch
		}
		if modelConfig.NUBatch != 0 {
			cfg.NUBatch = modelConfig.NUBatch
		}
		if modelConfig.NThreads != 0 {
			cfg.NThreads = modelConfig.NThreads
		}
		if modelConfig.NThreadsBatch != 0 {
			cfg.NThreadsBatch = modelConfig.NThreadsBatch
		}
		if modelConfig.CacheTypeK != 0 {
			cfg.CacheTypeK = modelConfig.CacheTypeK
		}
		if modelConfig.CacheTypeV != 0 {
			cfg.CacheTypeV = modelConfig.CacheTypeV
		}
		if modelConfig.FlashAttention != 0 {
			cfg.FlashAttention = modelConfig.FlashAttention
		}
		if modelConfig.UseDirectIO {
			cfg.UseDirectIO = modelConfig.UseDirectIO
		}
		if modelConfig.IgnoreIntegrityCheck {
			cfg.IgnoreIntegrityCheck = modelConfig.IgnoreIntegrityCheck
		}
		if modelConfig.NSeqMax != 0 {
			cfg.NSeqMax = modelConfig.NSeqMax
		}
		if modelConfig.OffloadKQV != nil {
			cfg.OffloadKQV = modelConfig.OffloadKQV
		}
		if modelConfig.OpOffload != nil {
			cfg.OpOffload = modelConfig.OpOffload
		}
		if modelConfig.NGpuLayers != nil {
			cfg.NGpuLayers = modelConfig.NGpuLayers
		}
		if modelConfig.SplitMode != 0 {
			cfg.SplitMode = modelConfig.SplitMode
		}
		if modelConfig.SystemPromptCache {
			cfg.SystemPromptCache = modelConfig.SystemPromptCache
		}
		if modelConfig.FirstMessageCache {
			cfg.FirstMessageCache = modelConfig.FirstMessageCache
		}
		if modelConfig.CacheMinTokens != 0 {
			cfg.CacheMinTokens = modelConfig.CacheMinTokens
		}
	}

	return cfg, nil
}

// =============================================================================

func (c *Catalog) buildIndex() error {
	c.biMutex.Lock()
	defer c.biMutex.Unlock()

	entries, err := os.ReadDir(c.catalogPath)
	if err != nil {
		return fmt.Errorf("build-index: read catalog dir: %w", err)
	}

	index := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		if entry.Name() == indexFile {
			continue
		}

		filePath := filepath.Join(c.catalogPath, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("build-index: read file name[%s]: %w", entry.Name(), err)
		}

		var catModels CatalogModels
		if err := yaml.Unmarshal(data, &catModels); err != nil {
			return fmt.Errorf("build-index: unmarshal name[%s]: %w", entry.Name(), err)
		}

		for _, model := range catModels.Models {
			index[model.ID] = entry.Name()
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("build-index: marshal index: %w", err)
	}

	indexPath := filepath.Join(c.catalogPath, indexFile)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("build-index: write index file: %w", err)
	}

	return nil
}

func (c *Catalog) loadIndex() (map[string]string, error) {
	indexPath := filepath.Join(c.catalogPath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if err := c.buildIndex(); err != nil {
			return nil, fmt.Errorf("load-index: build-index: %w", err)
		}

		data, err = os.ReadFile(indexPath)
		if err != nil {
			return nil, fmt.Errorf("load-index: read-index: %w", err)
		}
	}

	var index map[string]string
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("load-index: unmarshal-index: %w", err)
	}

	return index, nil
}
