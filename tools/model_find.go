package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelPath returns file path information about a model.
type ModelPath struct {
	ModelFile  string
	ProjFile   string
	Downloaded bool
}

// FindModel locates the physical location on disk and returns the full path.
func FindModel(modelPath string, modelID string) (ModelPath, error) {
	entries, err := os.ReadDir(modelPath)
	if err != nil {
		return ModelPath{}, fmt.Errorf("reading models directory: %w", err)
	}

	projID := fmt.Sprintf("mmproj-%s", modelID)

	var fi ModelPath

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

				id := strings.TrimSuffix(fileEntry.Name(), filepath.Ext(fileEntry.Name()))

				if id == modelID {
					fi.ModelFile = filepath.Join(modelPath, org, model, fileEntry.Name())
					continue
				}

				if id == projID {
					fi.ProjFile = filepath.Join(modelPath, org, model, fileEntry.Name())
					continue
				}
			}
		}
	}

	if fi.ModelFile == "" {
		return ModelPath{}, fmt.Errorf("model %q not found", modelID)
	}

	return fi, nil
}

func MustFindModel(modelPath string, modelName string) ModelPath {
	fi, err := FindModel(modelPath, modelName)
	if err != nil {
		panic(err.Error())
	}

	return fi
}
