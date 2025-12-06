package tools

import (
	"fmt"
	"os"
	"time"
)

// ModelFile provides information about a model.
type ModelFile struct {
	Organization string
	ModelName    string
	ModelFile    string
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
			model := modelEntry.Name()

			fileEntries, err := os.ReadDir(fmt.Sprintf("%s/%s/%s", modelPath, org, model))
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

				info, err := fileEntry.Info()
				if err != nil {
					continue
				}

				list = append(list, ModelFile{
					Organization: org,
					ModelName:    model,
					ModelFile:    fileEntry.Name(),
					Size:         info.Size(),
					Modified:     info.ModTime(),
				})
			}
		}
	}

	return list, nil
}
