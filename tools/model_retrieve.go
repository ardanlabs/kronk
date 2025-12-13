package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
	"go.yaml.in/yaml/v2"
)

// ModelFile provides information about a model.
type ModelFile struct {
	ID          string
	OwnedBy     string
	ModelFamily string
	Size        int64
	Modified    time.Time
}

// RetrieveModelFiles returns all the models in the given model directory.
func RetrieveModelFiles(modelBasePath string) ([]ModelFile, error) {
	var list []ModelFile

	index, err := loadIndex(modelBasePath)
	if err != nil {
		return nil, fmt.Errorf("unable to load index: %w", err)
	}

	for modelID, mp := range index {
		info, err := os.Stat(mp.ModelFile)
		if err != nil {
			return nil, fmt.Errorf("stat: %w", err)
		}

		modelPath := strings.TrimLeft(mp.ModelFile, modelBasePath)
		parts := strings.Split(modelPath, "/")

		mf := ModelFile{
			ID:          modelID,
			OwnedBy:     parts[0],
			ModelFamily: parts[1],
			Size:        info.Size(),
			Modified:    info.ModTime(),
		}

		list = append(list, mf)
	}

	slices.SortFunc(list, func(a, b ModelFile) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})

	return list, nil
}

// retrieveModelFile finds the model and returns the model file information.
func retrieveModelFile(modelBasePath string, modelID string) (ModelFile, error) {
	mp, err := RetrieveModelPath(modelBasePath, modelID)
	if err != nil {
		return ModelFile{}, fmt.Errorf("retrieve-model-path: %w", err)
	}

	info, err := os.Stat(mp.ModelFile)
	if err != nil {
		return ModelFile{}, fmt.Errorf("stat: %w", err)
	}

	modelPath := strings.TrimLeft(mp.ModelFile, modelBasePath)
	parts := strings.Split(modelPath, "/")

	mf := ModelFile{
		ID:          modelID,
		OwnedBy:     parts[0],
		ModelFamily: parts[1],
		Size:        info.Size(),
		Modified:    info.ModTime(),
	}

	return mf, nil
}

// =============================================================================

// ModelInfo provides all the model details.
type ModelInfo struct {
	ID      string
	Object  string
	Created int64
	OwnedBy string
	Details model.ModelInfo
}

// RetrieveModelInfo provides details for the specified model.
func RetrieveModelInfo(libPath string, modelBasePath string, modelID string) (ModelInfo, error) {
	modelID = strings.ToLower(modelID)

	mp, err := RetrieveModelPath(modelBasePath, modelID)
	if err != nil {
		return ModelInfo{}, err
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return ModelInfo{}, fmt.Errorf("show-model: unable to init kronk: %w", err)
	}

	const modelInstances = 1
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:      mp.ModelFile,
		ProjectionFile: mp.ProjFile,
	})

	if err != nil {
		return ModelInfo{}, fmt.Errorf("show-model: unable to load kronk: %w", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		krn.Unload(ctx)
	}()

	mf, err := retrieveModelFile(modelBasePath, modelID)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("show-model: unable to get model file information: %w", err)
	}

	mi := ModelInfo{
		ID:      mf.ID,
		Object:  "model",
		Created: mf.Modified.UnixMilli(),
		OwnedBy: mf.OwnedBy,
		Details: krn.ModelInfo(),
	}

	return mi, nil
}

// =============================================================================

// ModelPath returns file path information about a model.
type ModelPath struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// RetrieveModelPath locates the physical location on disk and returns the full path.
func RetrieveModelPath(modelBasePath string, modelID string) (ModelPath, error) {
	index, err := loadIndex(modelBasePath)
	if err != nil {
		return ModelPath{}, fmt.Errorf("load-index: %w", err)
	}

	modelID = strings.ToLower(modelID)

	modelPath, exists := index[modelID]
	if !exists {
		return ModelPath{}, fmt.Errorf("model %q not found", modelID)
	}

	return modelPath, nil
}

// MustRetrieveModel finds a model and panics if the model was not found. This
// should only be used for testing.
func MustRetrieveModel(modelBasePath string, modelID string) ModelPath {
	fi, err := RetrieveModelPath(modelBasePath, modelID)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// =============================================================================

// LoadIndex returns the catalog index.
func loadIndex(modelBasePath string) (map[string]ModelPath, error) {
	indexPath := filepath.Join(modelBasePath, indexFile)

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

	var index map[string]ModelPath
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("unmarshal-index: %w", err)
	}

	return index, nil
}
