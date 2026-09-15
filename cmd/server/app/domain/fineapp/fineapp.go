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
)

// DocumentPreparer extracts and stores an uploaded source document.
type DocumentPreparer interface {
	Extract(ctx context.Context, sourcePath string) (prepare.Artifact, error)
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

// ModelResolver locates an installed model without exposing its path through
// the HTTP API.
type ModelResolver interface {
	FullPath(modelID string) (backend.ModelPath, error)
}

type app struct {
	preparer           DocumentPreparer
	builder            DatasetBuilder
	preparations       *preparationOperations
	finetuner          FineTuner
	models             ModelResolver
	configuredModelIDs []string
	datasetsPath       string
	runtime            finetune.Runtime
}

func newApp(cfg Config) *app {
	configuredModelIDs := slices.Clone(cfg.ConfiguredModelIDs)
	slices.Sort(configuredModelIDs)

	return &app{
		preparer:           cfg.Preparer,
		builder:            cfg.Builder,
		preparations:       newPreparationOperations(),
		finetuner:          cfg.FineTuner,
		models:             cfg.Models,
		configuredModelIDs: configuredModelIDs,
		datasetsPath:       cfg.DatasetsPath,
		runtime:            cfg.Runtime,
	}
}

func (a *app) validatePreparationModel(id string) error {
	if !slices.Contains(a.configuredModelIDs, id) {
		return fmt.Errorf("model_id %q is not in the active model configuration", id)
	}
	return nil
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
