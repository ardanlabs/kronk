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
	Validated   bool
}

// RetrieveFiles returns all the models in the given model directory.
func (m *Models) RetrieveFiles() ([]File, error) {
	var list []File

	index := m.loadIndex()

	for modelID, mp := range index {
		if len(mp.ModelFiles) == 0 {
			continue
		}

		var totalSize int64
		var modified time.Time

		for _, f := range mp.ModelFiles {
			info, err := os.Stat(f)
			if err != nil {
				return nil, fmt.Errorf("stat: %w", err)
			}

			totalSize += info.Size()
			if info.ModTime().After(modified) {
				modified = info.ModTime()
			}
		}

		modelPath := strings.TrimPrefix(mp.ModelFiles[0], m.modelsPath)
		modelPath = strings.TrimPrefix(modelPath, string(filepath.Separator))
		parts := strings.Split(modelPath, string(filepath.Separator))

		var ownedBy string
		var modelFamily string

		if len(parts) > 0 {
			ownedBy = parts[0]
		}

		if len(parts) > 1 {
			modelFamily = parts[1]
		}

		mf := File{
			ID:          modelID,
			OwnedBy:     ownedBy,
			ModelFamily: modelFamily,
			Size:        totalSize,
			Modified:    modified,
			Validated:   mp.Validated,
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
		return File{}, fmt.Errorf("retrieve-file: missing model id")
	}

	mp, err := m.RetrievePath(modelID)
	if err != nil {
		return File{}, fmt.Errorf("retrieve-file: unable to retrieve path: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return File{}, fmt.Errorf("retrieve-file: no model files found")
	}

	var totalSize int64
	var modified time.Time

	for _, f := range mp.ModelFiles {
		info, err := os.Stat(f)
		if err != nil {
			return File{}, fmt.Errorf("stat: %w", err)
		}

		totalSize += info.Size()
		if info.ModTime().After(modified) {
			modified = info.ModTime()
		}
	}

	modelPath := strings.TrimPrefix(mp.ModelFiles[0], m.modelsPath)
	modelPath = strings.TrimPrefix(modelPath, string(filepath.Separator))
	parts := strings.Split(modelPath, string(filepath.Separator))

	var ownedBy string
	var modelFamily string

	if len(parts) > 0 {
		ownedBy = parts[0]
	}

	if len(parts) > 1 {
		modelFamily = parts[1]
	}

	mf := File{
		ID:          modelID,
		OwnedBy:     ownedBy,
		ModelFamily: modelFamily,
		Size:        totalSize,
		Modified:    modified,
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
	mf, err := m.retrieveFile(modelID)
	if err != nil {
		return Info{}, fmt.Errorf("retrieve-info: unable to get model file information: %w", err)
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
	ModelFiles []string `yaml:"model_files"`
	ProjFile   string   `yaml:"proj_file"`
	Downloaded bool     `yaml:"downloaded"`
	Validated  bool     `yaml:"validated"`
}

// RetrievePath locates the physical location on disk and returns the full path.
func (m *Models) RetrievePath(modelID string) (Path, error) {
	index := m.loadIndex()

	modelPath, exists := index[modelID]
	if !exists {
		return Path{}, fmt.Errorf("retrieve-path: model %q not found", modelID)
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
func (m *Models) loadIndex() map[string]Path {
	m.biMutex.Lock()
	defer m.biMutex.Unlock()

	indexPath := filepath.Join(m.modelsPath, indexFile)

	data, err := os.ReadFile(indexPath)
	if err != nil {
		return make(map[string]Path)
	}

	var index map[string]Path
	if err := yaml.Unmarshal(data, &index); err != nil {
		return make(map[string]Path)
	}

	return index
}
