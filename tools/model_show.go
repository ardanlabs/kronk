package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
)

// ModelInfo provides all the model details.
type ModelInfo struct {
	ID      string
	Object  string
	Created int64
	OwnedBy string
	Details model.ModelInfo
}

// ShowModel provides details for the specified model.
func ShowModel(libPath string, modelBasePath string, modelID string) (ModelInfo, error) {
	modelID = strings.ToLower(modelID)

	fi, err := FindModel(modelBasePath, modelID)
	if err != nil {
		return ModelInfo{}, err
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return ModelInfo{}, fmt.Errorf("show-model:unable to init kronk: %w", err)
	}

	const modelInstances = 1
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:      fi.ModelFile,
		ProjectionFile: fi.ProjFile,
	})

	if err != nil {
		return ModelInfo{}, fmt.Errorf("show-model:unable to load kronk: %w", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		krn.Unload(ctx)
	}()

	models, err := ListModels(modelBasePath)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("show-model:unable to get model file information: %w", err)
	}

	var modelFile ModelFile
	for _, model := range models {
		id := strings.ToLower(model.ID)
		if id == modelID {
			modelFile = model
			break
		}
	}

	mi := ModelInfo{
		ID:      modelFile.ID,
		Object:  "model",
		Created: modelFile.Modified.UnixMilli(),
		OwnedBy: modelFile.OwnedBy,
		Details: krn.ModelInfo(),
	}

	return mi, nil
}
