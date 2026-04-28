// Package show provides the catalog show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	id := args[0]

	url, err := client.DefaultURL(fmt.Sprintf("/v1/catalog/%s", id))
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

	var detail toolapp.CatalogDetailResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &detail); err != nil {
		return fmt.Errorf("do: unable to get catalog entry: %w", err)
	}

	print(detail.CatalogDetail)

	return nil
}

func runLocal(mdls *models.Models, args []string) error {
	id := args[0]

	entry, ok, err := mdls.CatalogEntry(id)
	if err != nil {
		return fmt.Errorf("lookup catalog entry: %w", err)
	}
	if !ok {
		return fmt.Errorf("catalog entry %q not found", id)
	}

	downloaded, validated := mdls.IndexState()

	detail := models.CatalogDetail{
		CatalogSummary: models.NewSummary(id, entry, downloaded, validated),
		Files:          models.NewFiles(entry),
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	data, err := mdls.GGUFHead(ctx, entry)
	if err == nil {
		metadata, perr := models.ParseGGUFMetadata(data)
		if perr == nil {
			detail.ModelMetadata = metadata
			detail.GGUFArch = metadata["general.architecture"]
			detail.ParameterCount = models.ParameterCount(metadata)
			detail.Parameters = models.FormatParameterCount(detail.ParameterCount)
			detail.Template = models.TemplateName(metadata)
			detail.Capabilities = models.CapabilitiesFor(metadata, entry.MMProj != "")
		}
	}

	print(detail)

	return nil
}

// =============================================================================

func print(d models.CatalogDetail) {
	fmt.Println()
	fmt.Println("Catalog Entry")
	fmt.Println("=============")
	fmt.Printf("ID:             %s\n", d.ID)
	fmt.Printf("Owned By:       %s\n", d.OwnedBy)
	fmt.Printf("Family:         %s\n", d.ModelFamily)
	fmt.Printf("Revision:       %s\n", d.Revision)
	fmt.Printf("Web Page:       %s\n", d.WebPage)
	fmt.Printf("Total Size:     %s\n", d.TotalSize)
	fmt.Printf("Has Projection: %t\n", d.HasProjection)
	fmt.Printf("Downloaded:     %t\n", d.Downloaded)
	fmt.Printf("Validated:      %t\n", d.Validated)

	if d.GGUFArch != "" || d.Parameters != "" || d.Template != "" {
		fmt.Println()
		fmt.Println("GGUF")
		fmt.Println("----")
		if d.GGUFArch != "" {
			fmt.Printf("Architecture:   %s\n", d.GGUFArch)
		}
		if d.Parameters != "" {
			fmt.Printf("Parameters:     %s\n", d.Parameters)
		}
		if d.Template != "" {
			fmt.Printf("Template:       %s\n", d.Template)
		}
	}

	fmt.Println()
	fmt.Println("Files")
	fmt.Println("-----")
	for _, f := range d.Files.Model {
		fmt.Printf("model:  %s (%s)\n", f.URL, models.FormatBytes(f.Size))
	}
	if d.Files.Proj.URL != "" {
		fmt.Printf("proj:   %s (%s)\n", d.Files.Proj.URL, models.FormatBytes(d.Files.Proj.Size))
	}

	if d.Capabilities.Endpoint != "" {
		fmt.Println()
		fmt.Println("Capabilities")
		fmt.Println("------------")
		fmt.Printf("Endpoint:   %s\n", d.Capabilities.Endpoint)
		fmt.Printf("Streaming:  %t\n", d.Capabilities.Streaming)
		fmt.Printf("Reasoning:  %t\n", d.Capabilities.Reasoning)
		fmt.Printf("Tooling:    %t\n", d.Capabilities.Tooling)
		fmt.Printf("Embedding:  %t\n", d.Capabilities.Embedding)
		fmt.Printf("Rerank:     %t\n", d.Capabilities.Rerank)
		fmt.Printf("Images:     %t\n", d.Capabilities.Images)
		fmt.Printf("Audio:      %t\n", d.Capabilities.Audio)
		fmt.Printf("Video:      %t\n", d.Capabilities.Video)
	}

	if len(d.ModelMetadata) > 0 {
		fmt.Println()
		fmt.Println("Model Metadata")
		fmt.Println("--------------")
		for k, v := range d.ModelMetadata {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}
