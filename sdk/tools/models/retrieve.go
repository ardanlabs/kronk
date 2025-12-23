package models

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

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
func (m *Models) RetrieveFiles() ([]File, error) {
	var list []File

	index, err := m.loadIndex()
	if err != nil {
		return nil, fmt.Errorf("unable to load index: %w", err)
	}

	for modelID, mp := range index {
		info, err := os.Stat(mp.ModelFile)
		if err != nil {
			return nil, fmt.Errorf("stat: %w", err)
		}

		modelPath := strings.TrimLeft(mp.ModelFile, m.modelsPath)
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
func (m *Models) retrieveFile(modelID string) (File, error) {
	if modelID == "" {
		return File{}, fmt.Errorf("missing model id")
	}

	mp, err := m.RetrievePath(modelID)
	if err != nil {
		return File{}, fmt.Errorf("retrieve-model-path: %w", err)
	}

	info, err := os.Stat(mp.ModelFile)
	if err != nil {
		return File{}, fmt.Errorf("stat: %w", err)
	}

	modelPath := strings.TrimLeft(mp.ModelFile, m.modelsPath)
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
}

// RetrieveInfo provides details for the specified model.
func (m *Models) RetrieveInfo(modelID string) (Info, error) {
	modelID = strings.ToLower(modelID)

	mf, err := m.retrieveFile(modelID)
	if err != nil {
		return Info{}, fmt.Errorf("show-model: unable to get model file information: %w", err)
	}

	mi := Info{
		ID:      mf.ID,
		Object:  "model",
		Created: mf.Modified.UnixMilli(),
		OwnedBy: mf.OwnedBy,
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
func (m *Models) RetrievePath(modelID string) (Path, error) {
	index, err := m.loadIndex()
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
func (m *Models) MustRetrieveModel(modelID string) Path {
	fi, err := m.RetrievePath(modelID)
	if err != nil {
		panic(err.Error())
	}

	return fi
}

// =============================================================================

// LoadIndex returns the catalog index.
func (m *Models) loadIndex() (map[string]Path, error) {
	indexPath := filepath.Join(m.modelsPath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		if err := m.BuildIndex(); err != nil {
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
