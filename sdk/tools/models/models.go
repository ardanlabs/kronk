// Package models provides support for tooling around model management.
package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"
)

var biMutex sync.Mutex

// BuildIndex builds the model index for fast model access.
func BuildIndex(modelBasePath string) error {
	biMutex.Lock()
	defer biMutex.Unlock()

	if err := removeEmptyDirs(modelBasePath); err != nil {
		return fmt.Errorf("remove-empty-dirs: %w", err)
	}

	entries, err := os.ReadDir(modelBasePath)
	if err != nil {
		return fmt.Errorf("list-models: reading models directory: %w", err)
	}

	index := make(map[string]Path)

	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		org := orgEntry.Name()

		modelEntries, err := os.ReadDir(fmt.Sprintf("%s/%s", modelBasePath, org))
		if err != nil {
			continue
		}

		for _, modelEntry := range modelEntries {
			if !modelEntry.IsDir() {
				continue
			}

			modelFamily := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", modelBasePath, org, modelFamily))
			if err != nil {
				continue
			}

			modelfiles := make(map[string]string)
			projFiles := make(map[string]string)

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}

				name := fileEntry.Name()

				if name == ".DS_Store" {
					continue
				}

				if strings.HasPrefix(name, "mmproj") {
					modelID := extractModelID(name[7:])
					projFiles[modelID] = filepath.Join(modelBasePath, org, modelFamily, fileEntry.Name())
					continue
				}

				modelID := extractModelID(fileEntry.Name())
				modelfiles[modelID] = filepath.Join(modelBasePath, org, modelFamily, fileEntry.Name())
			}

			for modelID, modelFile := range modelfiles {
				mp := Path{
					ModelFile:  modelFile,
					Downloaded: true,
				}

				if projFile, exists := projFiles[modelID]; exists {
					mp.ProjFile = projFile
				}

				modelID = strings.ToLower(modelID)
				index[modelID] = mp
			}
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	indexPath := filepath.Join(modelBasePath, indexFile)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}

func removeEmptyDirs(modelBasePath string) error {
	var dirs []string

	err := filepath.WalkDir(modelBasePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && path != modelBasePath {
			dirs = append(dirs, path)
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("walking directory tree: %w", err)
	}

	for i := len(dirs) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(dirs[i])
		if err != nil {
			continue
		}

		if isDirEffectivelyEmpty(entries) {
			// Remove any .DS_Store before removing directory
			dsStore := filepath.Join(dirs[i], ".DS_Store")
			os.Remove(dsStore)
			os.Remove(dirs[i])
		}
	}

	return nil
}

// isDirEffectivelyEmpty returns true if directory only contains ignorable files like .DS_Store
func isDirEffectivelyEmpty(entries []os.DirEntry) bool {
	for _, e := range entries {
		if e.Name() != ".DS_Store" {
			return false
		}
	}

	return true
}
