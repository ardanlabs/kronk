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
	"github.com/ardanlabs/kronk/sdk/kronk/model"
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

	catDetails, err := catalog.Details(modelID)
	if err != nil {
		return fmt.Errorf("retrieve-model-details: %w", err)
	}

	var mi *models.ModelInfo
	miTmp, err := mdls.ModelInformation(modelID)
	if err == nil {
		mi = &miTmp
	}

	rmc := catalog.ResolvedModelConfig(modelID)

	var vram *models.VRAM
	vramTmp, err := catalog.CalculateVRAM(modelID, rmc)
	if err == nil {
		vram = &vramTmp
	}

	printLocal(catDetails, rmc, mi, vram)

	return nil
}

// =============================================================================

func printWeb(model toolapp.CatalogModelResponse) {
	fmt.Println()
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:           %s\n", model.ID)
	fmt.Printf("Category:     %s\n", model.Category)
	fmt.Printf("Owned By:     %s\n", model.OwnedBy)
	fmt.Printf("Model Family: %s\n", model.ModelFamily)
	fmt.Printf("Architecture: %s\n", model.Architecture)
	fmt.Printf("GGUF Arch:    %s\n", model.GGUFArch)
	fmt.Printf("Web Page:     %s\n", models.NormalizeHuggingFaceURL(model.WebPage))
	fmt.Printf("Gated Model:  %t\n", model.GatedModel)
	fmt.Println()

	if model.VRAM != nil {
		fmt.Println("VRAM Requirements")
		fmt.Println("-----------------")
		fmt.Printf("KV Per Token/Layer: %s\n", formatBytes(model.VRAM.KVPerTokenPerLayer))
		fmt.Printf("KV Per Slot:        %s\n", formatBytes(model.VRAM.KVPerSlot))
		fmt.Printf("Slot Memory:        %s\n", formatBytes(model.VRAM.SlotMemory))
		fmt.Printf("Total VRAM:         %s\n", formatBytes(model.VRAM.TotalVRAM))
		fmt.Println()
	}

	fmt.Println("Files")
	fmt.Println("-----")

	for _, model := range model.Files.Models {
		fmt.Printf("Model:        %s (%s)\n", models.NormalizeHuggingFaceDownloadURL(model.URL), model.Size)
	}

	if model.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", models.NormalizeHuggingFaceDownloadURL(model.Files.Proj.URL), model.Files.Proj.Size)
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
		fmt.Printf("Devices:              %s\n", model.ModelConfig.Devices)
		fmt.Printf("Context Window:       %s\n", formatIntPtr(model.ModelConfig.ContextWindow))
		fmt.Printf("NBatch:               %s\n", formatIntPtr(model.ModelConfig.NBatch))
		fmt.Printf("NUBatch:              %s\n", formatIntPtr(model.ModelConfig.NUBatch))
		fmt.Printf("NThreads:             %s\n", formatIntPtr(model.ModelConfig.NThreads))
		fmt.Printf("NThreadsBatch:        %s\n", formatIntPtr(model.ModelConfig.NThreadsBatch))
		fmt.Printf("CacheTypeK:           %s\n", model.ModelConfig.CacheTypeK)
		fmt.Printf("CacheTypeV:           %s\n", model.ModelConfig.CacheTypeV)
		fmt.Printf("FlashAttention:       %v\n", model.ModelConfig.FlashAttention.String())
		fmt.Printf("NSeqMax:              %s\n", formatIntPtr(model.ModelConfig.NSeqMax))
		fmt.Printf("IncrementalCache:     %s\n", formatBoolPtr(model.ModelConfig.IncrementalCache))
		fmt.Printf("CacheMinTokens:       %s\n", formatIntPtr(model.ModelConfig.CacheMinTokens))
		if model.ModelConfig.RopeScaling.String() != "none" {
			fmt.Printf("RoPE Scaling:         %s\n", model.ModelConfig.RopeScaling)
			fmt.Printf("YaRN Orig Ctx:        %v\n", formatIntPtr(model.ModelConfig.PtrYarnOrigCtx))
			if model.ModelConfig.PtrRopeFreqBase != nil {
				fmt.Printf("RoPE Freq Base:       %g\n", *model.ModelConfig.PtrRopeFreqBase)
			}
			if model.ModelConfig.PtrYarnExtFactor != nil {
				fmt.Printf("YaRN Ext Factor:      %g\n", *model.ModelConfig.PtrYarnExtFactor)
			}
			if model.ModelConfig.PtrYarnAttnFactor != nil {
				fmt.Printf("YaRN Attn Factor:     %g\n", *model.ModelConfig.PtrYarnAttnFactor)
			}
		}
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

func printLocal(catDetails catalog.ModelDetails, rmc catalog.ModelConfig, mi *models.ModelInfo, vram *models.VRAM) {
	fmt.Println()
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:           %s\n", catDetails.ID)
	fmt.Printf("Category:     %s\n", catDetails.Category)
	fmt.Printf("Owned By:     %s\n", catDetails.OwnedBy)
	fmt.Printf("Model Family: %s\n", catDetails.ModelFamily)
	fmt.Printf("Architecture: %s\n", catDetails.Architecture)
	fmt.Printf("GGUF Arch:    %s\n", catDetails.GGUFArch)
	fmt.Printf("Web Page:     %s\n", models.NormalizeHuggingFaceURL(catDetails.WebPage))
	fmt.Println()

	if vram != nil {
		fmt.Println("VRAM Requirements")
		fmt.Println("-----------------")
		fmt.Printf("KV Per Token/Layer: %s\n", formatBytes(vram.KVPerTokenPerLayer))
		fmt.Printf("KV Per Slot:        %s\n", formatBytes(vram.KVPerSlot))
		fmt.Printf("Slot Memory:        %s\n", formatBytes(vram.SlotMemory))
		fmt.Printf("Total VRAM:         %s\n", formatBytes(vram.TotalVRAM))
		fmt.Println()
	}

	fmt.Println("Files")
	fmt.Println("-----")
	if len(catDetails.Files.Models) > 0 {
		for _, model := range catDetails.Files.Models {
			fmt.Printf("Model:        %s (%s)\n", models.NormalizeHuggingFaceDownloadURL(model.URL), model.Size)
		}
	}

	if catDetails.Files.Proj.URL != "" {
		fmt.Printf("Proj:         %s (%s)\n", models.NormalizeHuggingFaceDownloadURL(catDetails.Files.Proj.URL), catDetails.Files.Proj.Size)
	}

	fmt.Println()
	fmt.Println("Capabilities")
	fmt.Println("------------")
	fmt.Printf("Endpoint:     %s\n", catDetails.Capabilities.Endpoint)
	fmt.Printf("Images:       %t\n", catDetails.Capabilities.Images)
	fmt.Printf("Audio:        %t\n", catDetails.Capabilities.Audio)
	fmt.Printf("Video:        %t\n", catDetails.Capabilities.Video)
	fmt.Printf("Streaming:    %t\n", catDetails.Capabilities.Streaming)
	fmt.Printf("Reasoning:    %t\n", catDetails.Capabilities.Reasoning)
	fmt.Printf("Tooling:      %t\n", catDetails.Capabilities.Tooling)
	fmt.Printf("Embedding:    %t\n", catDetails.Capabilities.Embedding)
	fmt.Printf("Rerank:       %t\n", catDetails.Capabilities.Rerank)
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	fmt.Printf("Created:      %s\n", catDetails.Metadata.Created.Format("2006-01-02"))
	fmt.Printf("Collections:  %s\n", catDetails.Metadata.Collections)
	fmt.Printf("Description:  %s\n", catDetails.Metadata.Description)
	fmt.Println()

	fmt.Println("Model Config")
	fmt.Println("------------")
	fmt.Printf("Devices:              %s\n", rmc.Devices)
	fmt.Printf("Context Window:       %s\n", formatIntPtr(rmc.PtrContextWindow))
	fmt.Printf("NBatch:               %s\n", formatIntPtr(rmc.PtrNBatch))
	fmt.Printf("NUBatch:              %s\n", formatIntPtr(rmc.PtrNUBatch))
	fmt.Printf("NThreads:             %s\n", formatIntPtr(rmc.PtrNThreads))
	fmt.Printf("NThreadsBatch:        %s\n", formatIntPtr(rmc.PtrNThreadsBatch))
	fmt.Printf("CacheTypeK:           %s\n", rmc.CacheTypeK)
	fmt.Printf("CacheTypeV:           %s\n", rmc.CacheTypeV)
	fmt.Printf("FlashAttention:       %v\n", model.DerefFlashAttention(rmc.FlashAttention))
	fmt.Printf("NSeqMax:              %s\n", formatIntPtr(rmc.PtrNSeqMax))
	fmt.Printf("IncrementalCache:     %s\n", formatBoolPtr(rmc.PtrIncrementalCache))
	fmt.Printf("CacheMinTokens:       %s\n", formatIntPtr(rmc.PtrCacheMinTokens))
	if rmc.RopeScaling.String() != "none" {
		fmt.Printf("RoPE Scaling:         %s\n", rmc.RopeScaling)
		fmt.Printf("YaRN Orig Ctx:        %v\n", formatIntPtr(rmc.PtrYarnOrigCtx))
		if rmc.PtrRopeFreqBase != nil {
			fmt.Printf("RoPE Freq Base:       %g\n", *rmc.PtrRopeFreqBase)
		}
		if rmc.PtrYarnExtFactor != nil {
			fmt.Printf("YaRN Ext Factor:      %g\n", *rmc.PtrYarnExtFactor)
		}
		if rmc.PtrYarnAttnFactor != nil {
			fmt.Printf("YaRN Attn Factor:     %g\n", *rmc.PtrYarnAttnFactor)
		}
	}
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

func formatBytes(b int64) string {
	const (
		kb int64 = 1000
		mb       = kb * 1000
		gb       = mb * 1000
	)

	switch {
	case b >= gb:
		return fmt.Sprintf("%.2f GB", float64(b)/float64(gb))
	case b >= mb:
		return fmt.Sprintf("%.2f MB", float64(b)/float64(mb))
	case b >= kb:
		return fmt.Sprintf("%.2f KB", float64(b)/float64(kb))
	default:
		return fmt.Sprintf("%d bytes", b)
	}
}

func formatIntPtr(p *int) string {
	if p == nil {
		return "auto"
	}
	return fmt.Sprintf("%d", *p)
}

func formatBoolPtr(p *bool) string {
	if p == nil {
		return "auto"
	}
	return fmt.Sprintf("%t", *p)
}
