// This example generates a PNG with a multi-file FLUX.2 model.
//
// Experimental: The Malina SDK public API is subject to change.
//
// Set MALINA_LIB and the three component paths before running:
//
//	MALINA_LIB=/path/to/libs \
//	MALINA_DIFFUSION_MODEL=/path/to/flux.gguf \
//	MALINA_VAE_MODEL=/path/to/ae.safetensors \
//	MALINA_LLM_MODEL=/path/to/qwen.gguf \
//	make example-malina-flux2
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
)

const outputFile = "malina-flux2.png"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	diffusion := os.Getenv("MALINA_DIFFUSION_MODEL")
	vae := os.Getenv("MALINA_VAE_MODEL")
	llm := os.Getenv("MALINA_LLM_MODEL")
	if diffusion == "" || vae == "" || llm == "" {
		return errors.New("MALINA_DIFFUSION_MODEL, MALINA_VAE_MODEL, and MALINA_LLM_MODEL are required")
	}

	if err := malina.Init(); err != nil {
		return fmt.Errorf("initialize Malina: %w", err)
	}

	m, err := malina.New(
		model.WithDiffusionModelPath(diffusion),
		model.WithVAEPath(vae),
		model.WithLLMPath(llm),
	)
	if err != nil {
		return fmt.Errorf("load FLUX.2 model: %w", err)
	}
	defer unload(m)

	params := model.NewGenerateParams()
	params.Prompt = "an orange cat on a tropical beach playing with oranges"
	params.NegativePrompt = "mascots, watermark, signature"
	params.Steps = 4

	fmt.Println("Generating FLUX.2 image")
	start := time.Now()
	image, err := m.Generate(context.Background(), params)
	if err != nil {
		return fmt.Errorf("generate image: %w", err)
	}
	if err := os.WriteFile(outputFile, image.PNG, 0644); err != nil {
		return fmt.Errorf("write %s: %w", outputFile, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\n", outputFile, image.Width, image.Height, time.Since(start).Round(time.Millisecond))

	return nil
}

func unload(m *malina.Malina) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Unload(ctx); err != nil {
		fmt.Printf("unload: %v\n", err)
	}
}
