// Package show provides the catalog show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	modelID := args[0]

	path := fmt.Sprintf("/v1/catalog/%s", modelID)

	url, err := client.DefaultURL(path)
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var model toolapp.CatalogModelResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &model); err != nil {
		return fmt.Errorf("do: unable to get model list: %w", err)
	}

	printWeb(model)

	return nil
}

func runLocal(mdls *models.Models, catalog *catalog.Catalog, args []string) error {
	modelID := args[0]

	catModelID, _, _ := strings.Cut(modelID, "/")

	model, err := catalog.RetrieveModelDetails(catModelID)
	if err != nil {
		return fmt.Errorf("retrieve-model-details: %w", err)
	}

	var mi *models.ModelInfo
	miTmp, err := mdls.RetrieveModelInfo(catModelID)
	if err == nil {
		mi = &miTmp
	}

	mc := catalog.RetrieveModelConfig(modelID)

	printLocal(model, mc, mi)

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
	fmt.Printf("Gated Model:  %t\n", model.GatedModel)
	fmt.Println()

	fmt.Println("Files")
	fmt.Println("-----")

	for _, model := range model.Files.Models {
		fmt.Printf("Model:        %s (%s)\n", model.URL, model.Size)
	}

	if model.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", model.Files.Proj.URL, model.Files.Proj.Size)
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
	fmt.Printf("Embedding:    %t\n", model.Capabilities.Embedding)
	fmt.Printf("Rerank:       %t\n", model.Capabilities.Rerank)
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	fmt.Printf("Created:      %s\n", model.Metadata.Created.Format("2006-01-02"))
	fmt.Printf("Collections:  %s\n", model.Metadata.Collections)
	fmt.Printf("Description:  %s\n", model.Metadata.Description)
	fmt.Println()

	if model.ModelConfig != nil {
		fmt.Println("Model Config")
		fmt.Println("------------")
		fmt.Printf("Device:               %s\n", model.ModelConfig.Device)
		fmt.Printf("Context Window:       %d\n", model.ModelConfig.ContextWindow)
		fmt.Printf("NBatch:               %d\n", model.ModelConfig.NBatch)
		fmt.Printf("NUBatch:              %d\n", model.ModelConfig.NUBatch)
		fmt.Printf("NThreads:             %d\n", model.ModelConfig.NThreads)
		fmt.Printf("NThreadsBatch:        %d\n", model.ModelConfig.NThreadsBatch)
		fmt.Printf("CacheTypeK:           %s\n", model.ModelConfig.CacheTypeK)
		fmt.Printf("CacheTypeV:           %s\n", model.ModelConfig.CacheTypeV)
		fmt.Printf("FlashAttention:       %v\n", model.ModelConfig.FlashAttention)
		fmt.Printf("NSeqMax:              %d\n", model.ModelConfig.NSeqMax)
		fmt.Printf("SystemPromptCache:    %t\n", model.ModelConfig.SystemPromptCache)
		fmt.Printf("FirstMessageCache:    %t\n", model.ModelConfig.FirstMessageCache)
		fmt.Printf("CacheMinTokens:       %d\n", model.ModelConfig.CacheMinTokens)
		fmt.Println()
	}

	if model.ModelMetadata != nil {
		fmt.Println("Model Metadata")
		fmt.Println("--------------")
		for k, v := range model.ModelMetadata {
			fmt.Printf("%s: %s\n", k, formatMetadataValue(v))
		}
		fmt.Println()
	}
}

func printLocal(model catalog.Model, mc catalog.ModelConfig, mi *models.ModelInfo) {
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
	if len(model.Files.Models) > 0 {
		for _, model := range model.Files.Models {
			fmt.Printf("Model:        %s (%s)\n", model.URL, model.Size)
		}
	}

	if model.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", model.Files.Proj.URL, model.Files.Proj.Size)
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
	fmt.Printf("Embedding:    %t\n", model.Capabilities.Embedding)
	fmt.Printf("Rerank:       %t\n", model.Capabilities.Rerank)
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	fmt.Printf("Created:      %s\n", model.Metadata.Created.Format("2006-01-02"))
	fmt.Printf("Collections:  %s\n", model.Metadata.Collections)
	fmt.Printf("Description:  %s\n", model.Metadata.Description)
	fmt.Println()

	fmt.Println("Model Config")
	fmt.Println("------------")
	fmt.Printf("Device:               %s\n", mc.Device)
	fmt.Printf("Context Window:       %d\n", mc.ContextWindow)
	fmt.Printf("NBatch:               %d\n", mc.NBatch)
	fmt.Printf("NUBatch:              %d\n", mc.NUBatch)
	fmt.Printf("NThreads:             %d\n", mc.NThreads)
	fmt.Printf("NThreadsBatch:        %d\n", mc.NThreadsBatch)
	fmt.Printf("CacheTypeK:           %s\n", mc.CacheTypeK)
	fmt.Printf("CacheTypeV:           %s\n", mc.CacheTypeV)
	fmt.Printf("FlashAttention:       %v\n", mc.FlashAttention)
	fmt.Printf("NSeqMax:              %d\n", mc.NSeqMax)
	fmt.Printf("SystemPromptCache:    %t\n", mc.SystemPromptCache)
	fmt.Printf("FirstMessageCache:    %t\n", mc.FirstMessageCache)
	fmt.Printf("CacheMinTokens:       %d\n", mc.CacheMinTokens)
	fmt.Println()

	if mi != nil && mi.Metadata != nil {
		fmt.Println("Model Metadata")
		fmt.Println("--------------")
		for k, v := range mi.Metadata {
			fmt.Printf("%s: %s\n", k, formatMetadataValue(v))
		}
		fmt.Println()
	}
}

func formatMetadataValue(value string) string {
	if len(value) < 2 || value[0] != '[' {
		return value
	}

	inner := value[1 : len(value)-1]
	elements := strings.Split(inner, " ")

	if len(elements) <= 6 {
		return value
	}

	first := elements[:3]

	return fmt.Sprintf("[%s, ...]", strings.Join(first, ", "))
}
