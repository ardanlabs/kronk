package vram

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	sdkmodels "github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	return fmt.Errorf("not implemented as we don't have access to the model metadata")
}

func runLocal(models *sdkmodels.Models, modelID string, contextLength uint64, kvCacheType string) error {
	mi, err := models.RetrieveInfo(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model info: %w", err)
	}

	mp, err := models.RetrievePath(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model path: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return fmt.Errorf("no model files found")
	}

	// Calculate the total size of all model files
	var totalSize int64
	for _, file := range mp.ModelFiles {
		info, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("unable to stat model file: %w", err)
		}
		totalSize += info.Size()
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	modelFilePath := mp.ModelFiles[0]

	modelMetadata, err := model.ParseGGUFFile(modelFilePath)
	if err != nil {
		return fmt.Errorf("unable to parse GGUF file: %w", err)
	}

	vramUsage := sdkmodels.CalculateVRAM(modelMetadata, contextLength, kvCacheType)

	fmt.Println("=== Model Information ===")
	fmt.Printf("ID: %s\n", mi.ID)
	fmt.Printf("Owned By: %s\n", mi.OwnedBy)
	fmt.Printf("Size: %s\n", FormatBytes(uint64(totalSize)))
	fmt.Printf("Architecture: %s\n", modelMetadata.Architecture)
	fmt.Printf("Name: %s\n", modelMetadata.Name)
	fmt.Printf("Context Length: %d\n", modelMetadata.ContextLength)
	fmt.Printf("Embedding Length: %d\n", modelMetadata.EmbeddingLength)
	fmt.Printf("Block Count: %d\n", modelMetadata.BlockCount)
	fmt.Printf("Head Count: %d\n", modelMetadata.HeadCount)
	fmt.Printf("Head Count KV: %d\n", modelMetadata.HeadCountKV)
	fmt.Printf("Expert Count: %d\n", modelMetadata.ExpertCount)
	fmt.Printf("Expert Used Count: %d\n", modelMetadata.ExpertUsedCount)
	fmt.Printf("RoPE Dimension Count: %d\n", modelMetadata.RopeDimensionCount)
	if modelMetadata.SizeLabel != "" {
		expertCount, paramCount, activeParamCount, err := ParseSizeLabel(modelMetadata.SizeLabel)
		if err == nil {
			fmt.Printf("Parsed Size - Expert Count: %d, Parameter Count: %s, Active Parameter Count: %s\n", expertCount, paramCount, activeParamCount)
		}
	}

	fmt.Println("\n=== VRAM Calculation ===")
	fmt.Printf("KV Cache Type: %s\n", kvCacheType)
	fmt.Printf("Estimated VRAM Usage: %s\n", FormatBytes(vramUsage))
	fmt.Printf("Context Length: %d tokens\n", contextLength)
	if contextLength > modelMetadata.ContextLength {
		fmt.Printf("Warning: Context length %d exceeds model's maximum context length %d\n", contextLength, modelMetadata.ContextLength)

		atMaxModelContextSize := FormatBytes(sdkmodels.CalculateVRAM(modelMetadata, modelMetadata.ContextLength, kvCacheType))
		fmt.Printf("VRAM Usage at Maximum Model Context Length: %s\n", atMaxModelContextSize)
	}

	fmt.Println("\n=== VRAM Usage for Common Context Lengths ===")
	commonContexts := []uint64{512, 768, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 98304, 131072}
	for _, ctxSize := range commonContexts {
		vram := sdkmodels.CalculateVRAM(modelMetadata, ctxSize, kvCacheType)
		fmt.Printf("Context %d: %s\n", ctxSize, FormatBytes(vram))
	}

	return nil
}

func FormatBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	}
	if bytes < 1024*1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
	return fmt.Sprintf("%.2f TB", float64(bytes)/(1024*1024*1024*1024))
}

func ParseSizeLabel(sizeLabel string) (expertCount uint32, parameterCount, activeParamCount string, err error) {
	switch {
	// Handle the expert count format like "8x7B"
	case strings.Contains(sizeLabel, "x"):
		parts := strings.SplitN(sizeLabel, "x", 2)
		if len(parts) < 2 {
			return 0, sizeLabel, "-", err
		}

		// Parse expert count
		if expertCountVal, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			expertCount = uint32(expertCountVal)
		}

		parameterPart := strings.Join(parts[1:], "x")
		return expertCount, parameterPart, activeParamCount, err

	// Handle the expert count format like "30B-A3B"
	case strings.Contains(sizeLabel, "-A"):
		parts := strings.SplitN(sizeLabel, "-A", 2)
		if len(parts) < 2 {
			return 0, sizeLabel, "-", err
		}

		// Parse expert count
		if expertCountVal, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			expertCount = uint32(expertCountVal)
		}

		parameterPart := parts[0]
		activeParamCount = parts[1]
		return expertCount, parameterPart, activeParamCount, err

	default:
		return 0, sizeLabel, "-", err
	}
}
