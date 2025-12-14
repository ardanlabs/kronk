// Package pull provides the catalog pull command code.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/defaults"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

// RunWeb executes the catalog pull command against the model server.
func RunWeb(args []string) error {
	modelID := args[0]

	path := fmt.Sprintf("/v1/catalog/pull/%s", modelID)

	url, err := client.DefaultURL(path)
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	cln := client.NewSSE[toolapp.PullResponse](client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ch := make(chan toolapp.PullResponse)
	if err := cln.Do(ctx, http.MethodPost, url, nil, ch); err != nil {
		return fmt.Errorf("do: unable to download model: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}

	fmt.Println()

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
