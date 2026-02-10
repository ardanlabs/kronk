// Package show provides the show command code.
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

	url, err := client.DefaultURL(fmt.Sprintf("/v1/models/%s", modelID))
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

	var info toolapp.ModelInfoResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("do: unable to get model information: %w", err)
	}

	printWeb(info)

	return nil
}

func runLocal(mdls *models.Models, cat *catalog.Catalog, args []string) error {
	modelID := args[0]

	fi, err := mdls.FileInformation(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model file info: %w", err)
	}
	fi.ID = modelID

	mi, err := mdls.ModelInformation(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model info: %w", err)
	}

	rmc := cat.ResolvedModelConfig(modelID)

	var vram *models.VRAM
	vramTmp, err := cat.CalculateVRAM(modelID, rmc)
	if err == nil {
		vram = &vramTmp
	}

	printLocal(fi, mi, rmc, vram)

	return nil
}

// =============================================================================

func printWeb(mi toolapp.ModelInfoResponse) {
	fmt.Println()
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:          %s\n", mi.ID)
	fmt.Printf("Object:      %s\n", mi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(mi.Created))
	fmt.Printf("OwnedBy:     %s\n", mi.OwnedBy)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %s\n", formatBytes(mi.Size))
	fmt.Printf("HasProj:     %t\n", mi.HasProjection)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPT)
	fmt.Println()

	if mi.VRAM != nil {
		fmt.Println("VRAM Requirements")
		fmt.Println("-----------------")
		fmt.Printf("KV Per Token/Layer: %s\n", formatBytes(mi.VRAM.KVPerTokenPerLayer))
		fmt.Printf("KV Per Slot:        %s\n", formatBytes(mi.VRAM.KVPerSlot))
		fmt.Printf("Total Slots:        %d\n", mi.VRAM.TotalSlots)
		fmt.Printf("Slot Memory:        %s\n", formatBytes(mi.VRAM.SlotMemory))
		fmt.Printf("Total VRAM:         %s\n", formatBytes(mi.VRAM.TotalVRAM))
		fmt.Println()
	}

	if mi.ModelConfig != nil {
		fmt.Println("Model Configuration")
		fmt.Println("-------------------")
		fmt.Printf("Context Window:    %d\n", mi.ModelConfig.ContextWindow)
		fmt.Printf("Batch Size:        %d\n", mi.ModelConfig.NBatch)
		fmt.Printf("Micro Batch Size:  %d\n", mi.ModelConfig.NUBatch)
		fmt.Printf("Max Sequences:     %d\n", mi.ModelConfig.NSeqMax)
		fmt.Printf("Cache Type K:      %s\n", mi.ModelConfig.CacheTypeK)
		fmt.Printf("Cache Type V:      %s\n", mi.ModelConfig.CacheTypeV)
		fmt.Printf("System Prompt Cache: %t\n", mi.ModelConfig.SystemPromptCache)
		fmt.Printf("Incremental Cache:   %t\n", mi.ModelConfig.IncrementalCache)
		if mi.ModelConfig.RopeScaling.String() != "none" {
			fmt.Printf("RoPE Scaling:      %s\n", mi.ModelConfig.RopeScaling)
			fmt.Printf("YaRN Orig Ctx:     %v\n", formatIntPtr(mi.ModelConfig.YarnOrigCtx))
			if mi.ModelConfig.RopeFreqBase != nil {
				fmt.Printf("RoPE Freq Base:    %g\n", *mi.ModelConfig.RopeFreqBase)
			}
			if mi.ModelConfig.YarnExtFactor != nil {
				fmt.Printf("YaRN Ext Factor:   %g\n", *mi.ModelConfig.YarnExtFactor)
			}
			if mi.ModelConfig.YarnAttnFactor != nil {
				fmt.Printf("YaRN Attn Factor:  %g\n", *mi.ModelConfig.YarnAttnFactor)
			}
		}
		fmt.Println()
	}

	fmt.Println("Metadata")
	fmt.Println("--------")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

func printLocal(fi models.FileInfo, mi models.ModelInfo, rmc catalog.ModelConfig, vram *models.VRAM) {
	fmt.Println()
	fmt.Println("Model Details")
	fmt.Println("=============")
	fmt.Printf("ID:          %s\n", fi.ID)
	fmt.Printf("Object:      %s\n", fi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(fi.Created))
	fmt.Printf("OwnedBy:     %s\n", fi.OwnedBy)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %s\n", formatBytes(int64(mi.Size)))
	fmt.Printf("HasProj:     %t\n", mi.HasProjection)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPTModel)
	fmt.Printf("IsEmbed:     %t\n", mi.IsEmbedModel)
	fmt.Printf("IsRerank:    %t\n", mi.IsRerankModel)
	fmt.Println()

	if vram != nil {
		fmt.Println("VRAM Requirements")
		fmt.Println("-----------------")
		fmt.Printf("KV Per Token/Layer: %s\n", formatBytes(vram.KVPerTokenPerLayer))
		fmt.Printf("KV Per Slot:        %s\n", formatBytes(vram.KVPerSlot))
		fmt.Printf("Total Slots:        %d\n", vram.TotalSlots)
		fmt.Printf("Slot Memory:        %s\n", formatBytes(vram.SlotMemory))
		fmt.Printf("Total VRAM:         %s\n", formatBytes(vram.TotalVRAM))
		fmt.Println()
	}

	fmt.Println("Model Configuration")
	fmt.Println("-------------------")
	fmt.Printf("Context Window:    %d\n", rmc.ContextWindow)
	fmt.Printf("Batch Size:        %d\n", rmc.NBatch)
	fmt.Printf("Micro Batch Size:  %d\n", rmc.NUBatch)
	fmt.Printf("Max Sequences:     %d\n", rmc.NSeqMax)
	fmt.Printf("Cache Type K:      %s\n", rmc.CacheTypeK)
	fmt.Printf("Cache Type V:      %s\n", rmc.CacheTypeV)
	fmt.Printf("System Prompt Cache: %t\n", rmc.SystemPromptCache)
	fmt.Printf("Incremental Cache:   %t\n", rmc.IncrementalCache)
	if rmc.RopeScaling.String() != "none" {
		fmt.Printf("RoPE Scaling:      %s\n", rmc.RopeScaling)
		fmt.Printf("YaRN Orig Ctx:     %v\n", formatIntPtr(rmc.YarnOrigCtx))
		if rmc.RopeFreqBase != nil {
			fmt.Printf("RoPE Freq Base:    %g\n", *rmc.RopeFreqBase)
		}
		if rmc.YarnExtFactor != nil {
			fmt.Printf("YaRN Ext Factor:   %g\n", *rmc.YarnExtFactor)
		}
		if rmc.YarnAttnFactor != nil {
			fmt.Printf("YaRN Attn Factor:  %g\n", *rmc.YarnAttnFactor)
		}
	}
	fmt.Println()

	fmt.Println("Metadata")
	fmt.Println("--------")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, formatMetadataValue(v))
	}
}

// =============================================================================

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
		kb int64 = 1024
		mb       = kb * 1024
		gb       = mb * 1024
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
