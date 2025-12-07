package tools

import (
	"fmt"
	"os"
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
func ListModels(modelBasePath string) ([]ModelFile, error) {
	entries, err := os.ReadDir(modelBasePath)
	if err != nil {
		return nil, fmt.Errorf("list-models:reading models directory: %w", err)
	}

	var list []ModelFile

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

				list = append(list, ModelFile{
					ID:           extractModelID(fileEntry.Name()),
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
