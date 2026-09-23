// Package fineapp provides the HTTP API for document preparation and fine-tuning.
package fineapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// DocumentPreparer extracts and stores an uploaded source document.
type DocumentPreparer interface {
	ExtractDocuments(ctx context.Context, sourcePaths []string, progress prepare.ExtractionProgressFunc) (prepare.Artifact, error)
}

// DatasetBuilder creates preview and complete training datasets.
type DatasetBuilder interface {
	Build(ctx context.Context, request prepare.PreviewRequest) (prepare.PreviewArtifact, error)
	BuildFull(ctx context.Context, request prepare.FullBuildRequest) (prepare.FullBuildArtifact, error)
}

// FineTuner manages local fine-tuning runs.
type FineTuner interface {
	Start(ctx context.Context, request finetune.Request, update finetune.UpdateFunc) (finetune.RunInfo, error)
	Resume(ctx context.Context, id string, update finetune.UpdateFunc) (finetune.RunInfo, error)
	Stop() error
	Current() (finetune.RunInfo, bool)
	History() ([]finetune.RunInfo, error)
}

// TrainingRecipeProvider reports the settings used by the fine-tuning backend
// for a resolved model file.
type TrainingRecipeProvider interface {
	Recipe(modelPath string) (finetune.Recipe, error)
}

// ModelResolver locates an installed model without exposing its path through
// the HTTP API.
type ModelResolver interface {
	FullPath(modelID string) (backend.ModelPath, error)
}

// PreparationModelCatalog reports catalog membership, installation state,
// and installed GGUF metadata used to select dataset preparation models.
type PreparationModelCatalog interface {
	Catalog() (models.Catalog, error)
	IndexState() (downloaded, validated map[string]bool)
	ModelInformation(modelID string) (models.ModelInfo, error)
}

type app struct {
	preparer                DocumentPreparer
	builder                 DatasetBuilder
	preparations            *preparationOperations
	finetuner               FineTuner
	recipes                 TrainingRecipeProvider
	models                  ModelResolver
	preparationModelCatalog PreparationModelCatalog
	datasetsPath            string
	runtime                 finetune.Runtime
}

func newApp(cfg Config) *app {
	return &app{
		preparer:                cfg.Preparer,
		builder:                 cfg.Builder,
		preparations:            newPreparationOperations(),
		finetuner:               cfg.FineTuner,
		recipes:                 cfg.Recipes,
		models:                  cfg.Models,
		preparationModelCatalog: cfg.PreparationModels,
		datasetsPath:            cfg.DatasetsPath,
		runtime:                 cfg.Runtime,
	}
}

func (a *app) validatePreparationModel(id string) error {
	available, err := a.preparationModelIDs()
	if err != nil {
		return err
	}
	if !slices.Contains(available, id) {
		return fmt.Errorf("model_id %q is not available for dataset preparation", id)
	}
	return nil
}

func (a *app) preparationModelIDs() ([]string, error) {
	if a.preparationModelCatalog == nil {
		return nil, fmt.Errorf("preparation model catalog is not configured")
	}

	catalog, err := a.preparationModelCatalog.Catalog()
	if err != nil {
		return nil, fmt.Errorf("load preparation model catalog: %w", err)
	}
	downloaded, validated := a.preparationModelCatalog.IndexState()

	available := make([]string, 0, len(catalog.Models))
	for id, entry := range catalog.Models {
		if !downloaded[id] || !validated[id] || !supportsDatasetPreparation(entry.Capabilities) {
			continue
		}

		info, err := a.preparationModelCatalog.ModelInformation(id)
		if err != nil || info.IsEmbedModel || info.IsRerankModel {
			continue
		}
		if !supportsDatasetPreparation(models.CapabilitiesFor(info.Metadata, info.HasProjection)) {
			continue
		}

		available = append(available, id)
	}

	slices.Sort(available)
	return available, nil
}

func supportsDatasetPreparation(capabilities models.CatalogCapabilities) bool {
	return capabilities.Endpoint == "chat_completion" &&
		!capabilities.Embedding &&
		!capabilities.Rerank &&
		!capabilities.Images &&
		!capabilities.Audio &&
		!capabilities.Video
}

func (a *app) datasetFile(id, name string) (string, error) {
	if err := validateResourceID(id); err != nil {
		return "", fmt.Errorf("dataset_id: %w", err)
	}
	if a.datasetsPath == "" {
		return "", fmt.Errorf("dataset storage is not configured")
	}

	directory := filepath.Join(a.datasetsPath, id)
	info, err := os.Lstat(directory)
	if err != nil {
		return "", fmt.Errorf("locate dataset %q: %w", id, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("dataset %q is not a managed directory", id)
	}

	path := filepath.Join(directory, name)
	info, err = os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("locate dataset %q file %q: %w", id, name, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("dataset %q file %q is not a regular file", id, name)
	}

	return path, nil
}

func (a *app) modelFile(id string) (string, error) {
	if id == "" {
		return "", fmt.Errorf("model_id is required")
	}
	if a.models == nil {
		return "", fmt.Errorf("model catalog is not configured")
	}

	modelPath, err := a.models.FullPath(id)
	if err != nil {
		return "", fmt.Errorf("resolve model_id %q: %w", id, err)
	}
	if len(modelPath.ModelFiles) != 1 || modelPath.ModelFiles[0] == "" {
		return "", fmt.Errorf("model_id %q must resolve to one model file", id)
	}

	return modelPath.ModelFiles[0], nil
}

func validateResourceID(id string) error {
	if id == "" {
		return fmt.Errorf("is required")
	}
	if strings.TrimSpace(id) != id || id == "." || id == ".." || filepath.Base(id) != id || strings.Contains(id, `\`) {
		return fmt.Errorf("is invalid")
	}
	return nil
}
