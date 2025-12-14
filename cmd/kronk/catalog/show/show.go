// Package show provides the catalog show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

// RunWeb executes the catalog show command against the model server.
func RunWeb(args []string) error {
	modelID := args[0]

	path := fmt.Sprintf("/v1/catalog/%s", modelID)

	url, err := client.DefaultURL(path)
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var model toolapp.CatalogModelResponse
	if err := client.Do(ctx, http.MethodGet, url, nil, &model); err != nil {
		return fmt.Errorf("do: unable to get model list: %w", err)
	}

	printWeb(model)

	return nil
}

// RunLocal executes the catalog show command locally.
func RunLocal(args []string) error {
	modelID := args[0]
	basePath := defaults.BaseDir("")

	model, err := catalog.RetrieveModelDetails(basePath, modelID)
	if err != nil {
		return fmt.Errorf("retrieve-model-details: %w", err)
	}

	print(model)

	return nil
}

// =============================================================================

func printWeb(model toolapp.CatalogModelResponse) {
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:           %s\n", model.ID)
	fmt.Printf("Category:     %s\n", model.Category)
	fmt.Printf("Owned By:     %s\n", model.OwnedBy)
	fmt.Printf("Model Family: %s\n", model.ModelFamily)
	fmt.Printf("Web Page:     %s\n", model.WebPage)
	fmt.Println()

	fmt.Println("Files")
	fmt.Println("-----")
	fmt.Printf("Model:        %s (%s)\n", model.Files.Model.URL, model.Files.Model.Size)
	if model.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", model.Files.Proj.URL, model.Files.Proj.Size)
	}
	if model.Files.Jinja.URL != "" {
		fmt.Printf("Jinja:        %s (%s)\n", model.Files.Jinja.URL, model.Files.Jinja.Size)
	}
	fmt.Println()

	fmt.Println("Capabilities")
	fmt.Println("------------")
	fmt.Printf("Endpoint:     %s\n", model.Capabilities.Endpoint)
	fmt.Printf("Images:       %t\n", model.Capabilities.Images)
	fmt.Printf("Audio:        %t\n", model.Capabilities.Audio)
	fmt.Printf("Video:        %t\n", model.Capabilities.Video)
	fmt.Printf("Streaming:    %t\n", model.Capabilities.Streaming)
	fmt.Printf("Reasoning:    %t\n", model.Capabilities.Reasoning)
	fmt.Printf("Tooling:      %t\n", model.Capabilities.Tooling)
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	fmt.Printf("Created:      %s\n", model.Metadata.Created.Format("2006-01-02"))
	fmt.Printf("Collections:  %s\n", model.Metadata.Collections)
	fmt.Printf("Description:  %s\n", model.Metadata.Description)

}

func print(model catalog.Model) {
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:           %s\n", model.ID)
	fmt.Printf("Category:     %s\n", model.Category)
	fmt.Printf("Owned By:     %s\n", model.OwnedBy)
	fmt.Printf("Model Family: %s\n", model.ModelFamily)
	fmt.Printf("Web Page:     %s\n", model.WebPage)
	fmt.Println()

	fmt.Println("Files")
	fmt.Println("-----")
	fmt.Printf("Model:        %s (%s)\n", model.Files.Model.URL, model.Files.Model.Size)
	if model.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", model.Files.Proj.URL, model.Files.Proj.Size)
	}
	if model.Files.Jinja.URL != "" {
		fmt.Printf("Jinja:        %s (%s)\n", model.Files.Jinja.URL, model.Files.Jinja.Size)
	}
	fmt.Println()

	fmt.Println("Capabilities")
	fmt.Println("------------")
	fmt.Printf("Endpoint:     %s\n", model.Capabilities.Endpoint)
	fmt.Printf("Images:       %t\n", model.Capabilities.Images)
	fmt.Printf("Audio:        %t\n", model.Capabilities.Audio)
	fmt.Printf("Video:        %t\n", model.Capabilities.Video)
	fmt.Printf("Streaming:    %t\n", model.Capabilities.Streaming)
	fmt.Printf("Reasoning:    %t\n", model.Capabilities.Reasoning)
	fmt.Printf("Tooling:      %t\n", model.Capabilities.Tooling)
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	fmt.Printf("Created:      %s\n", model.Metadata.Created.Format("2006-01-02"))
	fmt.Printf("Collections:  %s\n", model.Metadata.Collections)
	fmt.Printf("Description:  %s\n", model.Metadata.Description)

}
