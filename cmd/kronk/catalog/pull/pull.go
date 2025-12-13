// Package pull provides the catalog pull command code.
package pull

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
	"github.com/ardanlabs/kronk/tools/catalog"
)

// RunWeb executes the catalog pull command against the model server.
func RunWeb(args []string) error {
	modelID := args[0]
	_ = modelID

	fmt.Println("catalog pull: not implemented")
	return nil
}

// RunLocal executes the catalog pull command locally.
func RunLocal(args []string) error {
	modelID := args[0]

	basePath := defaults.BaseDir("")
	modelBasePath := defaults.ModelsDir("")

	model, err := catalog.RetrieveModelDetails(basePath, modelID)
	if err != nil {
		return fmt.Errorf("retrieve-model-details: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, err = tools.DownloadModel(ctx, kronk.FmtLogger, model.Files.Model.URL, model.Files.Proj.URL, modelBasePath)
	if err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}
