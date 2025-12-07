package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ModelFile provides information about a model.
type ModelFile struct {
	ID           string
	Organization string
	ModelFamily  string
	Size         int64
	Modified     time.Time
}

// ListModels lists all the models in the given directory.
func ListModels(modelPath string) ([]ModelFile, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return nil, fmt.Errorf("reading models directory: %w", err)
	}

	var list []ModelFile

	for _, orgEntry := range entries {
		if !orgEntry.IsDir() {
			continue
		}

		org := orgEntry.Name()

		modelEntries, err := os.ReadDir(fmt.Sprintf("%s/%s", modelPath, org))
		if err != nil {
			continue
		}

		for _, modelEntry := range modelEntries {
			if !modelEntry.IsDir() {
				continue
			}
			modelFamily := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", modelPath, org, modelFamily))
			if err != nil {
				continue
			}

			for _, fileEntry := range fileEntries {
				if fileEntry.IsDir() {
					continue
				}

				if fileEntry.Name() == ".DS_Store" {
					continue
				}

				if strings.HasPrefix(fileEntry.Name(), "mmproj") {
					continue
				}

				info, err := fileEntry.Info()
				if err != nil {
					continue
				}

				modelID := strings.TrimSuffix(fileEntry.Name(), filepath.Ext(fileEntry.Name()))

				list = append(list, ModelFile{
					ID:           modelID,
					Organization: org,
					ModelFamily:  modelFamily,
					Size:         info.Size(),
					Modified:     info.ModTime(),
				})
			}
		}
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
