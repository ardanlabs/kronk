package models

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"go.yaml.in/yaml/v2"
)

// File provides information about a model.
type File struct {
	ID          string
	OwnedBy     string
	ModelFamily string
	Size        int64
	Modified    time.Time
}

// RetrieveFiles returns all the models in the given model directory.
func RetrieveFiles(modelBasePath string) ([]File, error) {
	var list []File

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

		mf := File{
			ID:          modelID,
			OwnedBy:     parts[0],
			ModelFamily: parts[1],
			Size:        info.Size(),
			Modified:    info.ModTime(),
		}

		list = append(list, mf)
	}

	slices.SortFunc(list, func(a, b File) int {
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

// retrieveFile finds the model and returns the model file information.
func retrieveFile(modelBasePath string, modelID string) (File, error) {
	mp, err := RetrievePath(modelBasePath, modelID)
	if err != nil {
		return File{}, fmt.Errorf("retrieve-model-path: %w", err)
	}

	info, err := os.Stat(mp.ModelFile)
	if err != nil {
		return File{}, fmt.Errorf("stat: %w", err)
	}

	modelPath := strings.TrimLeft(mp.ModelFile, modelBasePath)
	parts := strings.Split(modelPath, "/")

	mf := File{
		ID:          modelID,
		OwnedBy:     parts[0],
		ModelFamily: parts[1],
		Size:        info.Size(),
		Modified:    info.ModTime(),
	}

	return mf, nil
}

// =============================================================================

// Info provides all the model details.
type Info struct {
	ID      string
	Object  string
	Created int64
	OwnedBy string
	Details model.ModelInfo
}

// RetrieveInfo provides details for the specified model.
func RetrieveInfo(libPath string, modelBasePath string, modelID string) (Info, error) {
	modelID = strings.ToLower(modelID)

	mp, err := RetrievePath(modelBasePath, modelID)
	if err != nil {
		return Info{}, err
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return Info{}, fmt.Errorf("show-model: unable to init kronk: %w", err)
	}

	const modelInstances = 1
	krn, err := kronk.New(modelInstances, &templater{}, model.Config{
		ModelFile: mp.ModelFile,
		ProjFile:  mp.ProjFile,
	})

	if err != nil {
		return Info{}, fmt.Errorf("show-model: unable to load kronk: %w", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		krn.Unload(ctx)
	}()

	mf, err := retrieveFile(modelBasePath, modelID)
	if err != nil {
		return Info{}, fmt.Errorf("show-model: unable to get model file information: %w", err)
	}

	mi := Info{
		ID:      mf.ID,
		Object:  "model",
		Created: mf.Modified.UnixMilli(),
		OwnedBy: mf.OwnedBy,
		Details: krn.ModelInfo(),
	}

	return mi, nil
}

// =============================================================================

// Path returns file path information about a model.
type Path struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// RetrievePath locates the physical location on disk and returns the full path.
func RetrievePath(modelBasePath string, modelID string) (Path, error) {
	index, err := loadIndex(modelBasePath)
	if err != nil {
		return Path{}, fmt.Errorf("load-index: %w", err)
	}

	modelID = strings.ToLower(modelID)

	modelPath, exists := index[modelID]
	if !exists {
		return Path{}, fmt.Errorf("model %q not found", modelID)
	}

	return modelPath, nil
}

// MustRetrieveModel finds a model and panics if the model was not found. This
// should only be used for testing.
func MustRetrieveModel(modelBasePath string, modelID string) Path {
	fi, err := RetrievePath(modelBasePath, modelID)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// =============================================================================

// LoadIndex returns the catalog index.
func loadIndex(modelBasePath string) (map[string]Path, error) {
	indexPath := filepath.Join(modelBasePath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if err := BuildIndex(modelBasePath); err != nil {
			return nil, fmt.Errorf("build-index: %w", err)
		}

		data, err = os.ReadFile(indexPath)
		if err != nil {
			return nil, fmt.Errorf("read-index: %w", err)
		}
	}

	var index map[string]Path
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("unmarshal-index: %w", err)
	}

	return index, nil
}

// =============================================================================

type templater struct{}

// Retrieve implement the templater interface for the construction of
// Kronk which is used to get model details.
func (t *templater) Retrieve(modelID string) (model.Template, error) {
	return model.Template{}, nil
}
