package catalog

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"go.yaml.in/yaml/v2"
)

// ModelList returns the collection of models in the catalog with
// some filtering capabilities.
func (c *Catalog) ModelList(filterCategory string) ([]ModelDetails, error) {
	catalogs, err := c.All()
	if err != nil {
		return nil, fmt.Errorf("catalog-model-list: catalog list: %w", err)
	}

	modelFiles, err := c.models.Files()
	if err != nil {
		return nil, fmt.Errorf("catalog-model-list: retrieve-model-files: %w", err)
	}

	validatedModels := make(map[string]struct{})

	for _, mf := range modelFiles {
		if mf.Validated {
			validatedModels[mf.ID] = struct{}{}
		}
	}

	var list []ModelDetails
	for _, cat := range catalogs {
		if filterCategory != "" && !strings.Contains(strings.ToLower(cat.Name), strings.ToLower(filterCategory)) {
			continue
		}

		for _, model := range cat.Models {
			_, validated := validatedModels[model.ID]
			model.Downloaded = validated
			model.Validated = validated

			list = append(list, model)
		}
	}

	slices.SortFunc(list, func(a, b ModelDetails) int {
		return cmp.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
	})

	return list, nil
}

// Details returns the model information for the specified model
// that is defined only in the catalog only.
func (c *Catalog) Details(modelID string) (ModelDetails, error) {
	modelID, _, _ = strings.Cut(modelID, "/")

	index, err := c.loadIndex()
	if err != nil {
		return ModelDetails{}, fmt.Errorf("retrieve-model-details: load-index: %w", err)
	}

	catalogFile := index[modelID]
	if catalogFile == "" {
		return ModelDetails{}, fmt.Errorf("retrieve-model-details: model[%s] not found in index", modelID)
	}

	catalog, err := c.singleCatalog(catalogFile)
	if err != nil {
		return ModelDetails{}, fmt.Errorf("retrieve-model-details: retrieving catalog: %w", err)
	}

	for _, model := range catalog.Models {
		if strings.EqualFold(model.ID, modelID) {
			modelFiles, err := c.models.Files()
			if err != nil {
				return ModelDetails{}, fmt.Errorf("retrieve-model-details: retrieving mode files: %w", err)
			}

			for _, mf := range modelFiles {
				if mf.ID == model.ID && mf.Validated {
					model.Downloaded = true
					model.Validated = true
				}
			}

			model.CatalogFile = catalogFile

			return model, nil
		}
	}

	return ModelDetails{}, fmt.Errorf("retrieve-model-details: model[%s] not found", modelID)
}

// All reads all the catalogs from a previous download.
func (c *Catalog) All() ([]CatalogModels, error) {
	files, err := c.catalogYAMLFiles()
	if err != nil {
		return nil, fmt.Errorf("retrieve-catalogs: %w", err)
	}

	var catalogs []CatalogModels

	for _, name := range files {
		catalog, err := c.singleCatalog(name)
		if err != nil {
			return nil, fmt.Errorf("retrieve-catalogs: retrieve-catalog name[%s]: %w", name, err)
		}

		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil
}

// ResolvedModelConfig reads the catalog and model config file for the
// specified model id and returns a ModelConfig with sampling values.
//
// The configuration is resolved through a three-tier priority system:
//
//  1. Model Analysis (lowest priority) — hardware-aware defaults derived from
//     the GGUF file metadata and available system hardware (GPU memory, etc.).
//     This sets context-window, nbatch, nubatch, cache-type-k/v, flash-attention,
//     and ngpu-layers. Skipped when the model is not downloaded.
//
//  2. Catalog YAML — model-specific settings from catalog files. These override
//     the analysis defaults for any non-zero fields.
//
//  3. model_config.yaml (highest priority) — user overrides that always win.
func (c *Catalog) ResolvedModelConfig(modelID string) ModelConfig {

	// Check the resolved cache first using a read lock.
	c.resolvedMu.RLock()
	if cfg, ok := c.resolvedConfig[modelID]; ok {
		c.resolvedMu.RUnlock()
		return cfg
	}
	c.resolvedMu.RUnlock()

	// Layer 1: Seed config from model analysis if the model is on disk.
	// This provides hardware-aware defaults for context window, batch sizes,
	// cache types, flash attention, and GPU layers.
	cfg := c.analysisDefaults(modelID)

	// Layer 2: Look in the catalog config for the specified model.
	var catalogFound bool
	catalog, err := c.Details(modelID)
	if err == nil {
		catalogFound = true
	}

	// Layer 3: Look in the model config for the specified model.
	modelConfig, modelCfgFound := c.modelConfig[modelID]

	// Apply catalog settings over analysis defaults.
	if catalogFound {
		mergeModelConfig(&cfg, catalog.BaseModelConfig)
		if catalog.Template != "" {
			cfg.Template = catalog.Template
		}
	}

	// Apply model config settings if found (overrides everything).
	if modelCfgFound {
		mergeModelConfig(&cfg, modelConfig)
	}

	// Store the resolved config in the cache.
	c.resolvedMu.Lock()
	c.resolvedConfig[modelID] = cfg
	c.resolvedMu.Unlock()

	return cfg
}

// KronkResolvedModelConfig reads the catalog and model config file for
// the specified model id and returns a model config for use with kronk.New().
func (c *Catalog) KronkResolvedModelConfig(modelID string) (model.Config, error) {

	// Get the file path for this model on disk. If this fails, the
	// model hasn't been downloaded and nothing else to do.
	fp, err := c.models.FullPath(modelID)
	if err != nil {
		return model.Config{}, fmt.Errorf("retrieve-model-config: unable to get model[%s] path: %w", modelID, err)
	}

	// Get the merged config from catalog and model_config.yaml.
	mc := c.ResolvedModelConfig(modelID)

	if err := c.ResolveGrammar(&mc.Sampling); err != nil {
		return model.Config{}, fmt.Errorf("kronk-resolved-model-config: %w", err)
	}

	// Convert to model.Config and set file paths.
	cfg := mc.ToKronkConfig()
	cfg.ModelFiles = fp.ModelFiles
	cfg.ProjFile = fp.ProjFile

	// Resolve draft model file paths if configured.
	if mc.DraftModel != nil && mc.DraftModel.ModelID != "" {
		draftPath, err := c.models.FullPath(mc.DraftModel.ModelID)
		if err != nil {
			return model.Config{}, fmt.Errorf("kronk-resolved-model-config: unable to get draft model[%s] path: %w", mc.DraftModel.ModelID, err)
		}
		if cfg.DraftModel == nil {
			cfg.DraftModel = &model.DraftModelConfig{}
		}
		cfg.DraftModel.ModelFiles = draftPath.ModelFiles
	}

	return cfg, nil
}

// ModelFullPath returns the file paths for a model by its ID.
func (c *Catalog) ModelFullPath(modelID string) (models.Path, error) {
	return c.models.FullPath(modelID)
}

// =============================================================================

func (c *Catalog) catalogYAMLFiles() ([]string, error) {
	entries, err := os.ReadDir(c.catalogPath)
	if err != nil {
		return nil, fmt.Errorf("catalog-yaml-files: read dir: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		if entry.Name() == indexFile {
			continue
		}
		files = append(files, entry.Name())
	}

	return files, nil
}

func (c *Catalog) singleCatalog(catalogFile string) (CatalogModels, error) {
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

func (c *Catalog) buildIndex() error {
	c.biMutex.Lock()
	defer c.biMutex.Unlock()

	files, err := c.catalogYAMLFiles()
	if err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	index := make(map[string]string)

	for _, name := range files {
		cat, err := c.singleCatalog(name)
		if err != nil {
			return fmt.Errorf("build-index: read catalog name[%s]: %w", name, err)
		}

		for _, model := range cat.Models {
			index[model.ID] = name
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

// analysisDefaults runs ModelAnalysis on the specified model and converts
// the balanced recommendation into a ModelConfig. If the model is not
// downloaded or analysis fails, an empty ModelConfig is returned so that
// the catalog and model_config.yaml layers provide all values (matching
// the previous behavior).
func (c *Catalog) analysisDefaults(modelID string) ModelConfig {
	analysis, err := c.models.ModelAnalysis(modelID)
	if err != nil {
		return ModelConfig{}
	}

	rec := analysis.Recommended

	var cfg ModelConfig

	cfg.PtrContextWindow = new(int(rec.ContextWindow))
	cfg.PtrNBatch = new(defAnalysisNBatch)
	cfg.PtrNUBatch = new(defAnalysisNUBatch)
	cfg.PtrNSeqMax = new(int(rec.NSeqMax))

	if k, err := model.ParseGGMLType(rec.CacheTypeK); err == nil {
		cfg.CacheTypeK = k
	}

	if v, err := model.ParseGGMLType(rec.CacheTypeV); err == nil {
		cfg.CacheTypeV = v
	}

	switch rec.FlashAttention {
	case "auto":
		cfg.FlashAttention = new(model.FlashAttentionAuto)
	case "disabled":
		cfg.FlashAttention = new(model.FlashAttentionDisabled)
	default:
		cfg.FlashAttention = new(model.FlashAttentionEnabled)
	}

	// model.Config: PtrNGpuLayers nil = all on GPU, 0 = all on GPU, -1 = all on CPU.
	// Only set when we explicitly want CPU-only.
	if rec.NGPULayers < 0 {
		n := int(rec.NGPULayers)
		cfg.PtrNGpuLayers = &n
	}

	return cfg
}

// Default batch sizes used when seeding from analysis.
const (
	defAnalysisNBatch  = 2048
	defAnalysisNUBatch = 512
)

// mergeModelConfig overlays non-zero fields from src onto dst.
func mergeModelConfig(dst *ModelConfig, src ModelConfig) {
	if src.Template != "" {
		dst.Template = src.Template
	}
	if src.PtrContextWindow != nil {
		dst.PtrContextWindow = src.PtrContextWindow
	}
	if src.PtrNBatch != nil {
		dst.PtrNBatch = src.PtrNBatch
	}
	if src.PtrNUBatch != nil {
		dst.PtrNUBatch = src.PtrNUBatch
	}
	if src.PtrNThreads != nil {
		dst.PtrNThreads = src.PtrNThreads
	}
	if src.PtrNThreadsBatch != nil {
		dst.PtrNThreadsBatch = src.PtrNThreadsBatch
	}
	if src.CacheTypeK != 0 {
		dst.CacheTypeK = src.CacheTypeK
	}
	if src.CacheTypeV != 0 {
		dst.CacheTypeV = src.CacheTypeV
	}
	if src.FlashAttention != nil {
		dst.FlashAttention = src.FlashAttention
	}
	if src.PtrUseDirectIO != nil {
		dst.PtrUseDirectIO = src.PtrUseDirectIO
	}
	if src.PtrUseMMap != nil {
		dst.PtrUseMMap = src.PtrUseMMap
	}
	if src.NUMA != "" {
		dst.NUMA = src.NUMA
	}
	if src.PtrNSeqMax != nil {
		dst.PtrNSeqMax = src.PtrNSeqMax
	}
	if src.PtrOffloadKQV != nil {
		dst.PtrOffloadKQV = src.PtrOffloadKQV
	}
	if src.PtrOpOffload != nil {
		dst.PtrOpOffload = src.PtrOpOffload
	}
	if src.PtrOpOffloadMinBatch != nil {
		dst.PtrOpOffloadMinBatch = src.PtrOpOffloadMinBatch
	}
	if src.PtrNGpuLayers != nil {
		dst.PtrNGpuLayers = src.PtrNGpuLayers
	}
	if src.PtrSplitMode != nil {
		dst.PtrSplitMode = src.PtrSplitMode
	}
	if len(src.TensorSplit) > 0 {
		dst.TensorSplit = src.TensorSplit
	}
	if len(src.TensorBuftOverrides) > 0 {
		dst.TensorBuftOverrides = src.TensorBuftOverrides
	}
	if src.PtrMainGPU != nil {
		dst.PtrMainGPU = src.PtrMainGPU
	}
	if len(src.Devices) > 0 {
		dst.Devices = src.Devices
	}
	if src.MoE != nil {
		dst.MoE = src.MoE
	}
	if src.PtrSWAFull != nil {
		dst.PtrSWAFull = src.PtrSWAFull
	}
	if src.PtrIncrementalCache != nil {
		dst.PtrIncrementalCache = src.PtrIncrementalCache
	}
	if src.PtrCacheMinTokens != nil {
		dst.PtrCacheMinTokens = src.PtrCacheMinTokens
	}
	if src.PtrCacheSlotTimeout != nil {
		dst.PtrCacheSlotTimeout = src.PtrCacheSlotTimeout
	}
	if src.PtrInsecureLogging != nil {
		dst.PtrInsecureLogging = src.PtrInsecureLogging
	}
	if src.RopeScaling != 0 {
		dst.RopeScaling = src.RopeScaling
	}
	if src.PtrRopeFreqBase != nil {
		dst.PtrRopeFreqBase = src.PtrRopeFreqBase
	}
	if src.PtrRopeFreqScale != nil {
		dst.PtrRopeFreqScale = src.PtrRopeFreqScale
	}
	if src.PtrYarnExtFactor != nil {
		dst.PtrYarnExtFactor = src.PtrYarnExtFactor
	}
	if src.PtrYarnAttnFactor != nil {
		dst.PtrYarnAttnFactor = src.PtrYarnAttnFactor
	}
	if src.PtrYarnBetaFast != nil {
		dst.PtrYarnBetaFast = src.PtrYarnBetaFast
	}
	if src.PtrYarnBetaSlow != nil {
		dst.PtrYarnBetaSlow = src.PtrYarnBetaSlow
	}
	if src.PtrYarnOrigCtx != nil {
		dst.PtrYarnOrigCtx = src.PtrYarnOrigCtx
	}
	if src.DraftModel != nil {
		dst.DraftModel = src.DraftModel
	}

	// Merge sampling: src overrides non-zero fields in dst.
	dst.Sampling = mergeSampling(dst.Sampling, src.Sampling)
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
