package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/model"
)

// ShowModel provides details for the specified model.
func ShowModel(libPath string, modelBasePath string, modelID string) (model.ModelInfo, error) {
	fi, err := FindModel(modelBasePath, modelID)
	if err != nil {
		return model.ModelInfo{}, err
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return model.ModelInfo{}, fmt.Errorf("show-model:unable to init kronk: %w", err)
	}

	const modelInstances = 1
	krn, err := kronk.New(modelInstances, model.Config{
		ModelFile:      fi.ModelFile,
		ProjectionFile: fi.ProjFile,
	})

	if err != nil {
		return model.ModelInfo{}, fmt.Errorf("show-model:unable to load kronk: %w", err)
	}

	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		krn.Unload(ctx)
	}()

	return krn.ModelInfo(), nil
}
