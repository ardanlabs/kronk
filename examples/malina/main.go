// This example generates a PNG with the Malina SDK.
// It uses a local stable-diffusion.cpp model and native library.
//
// Experimental: The Malina SDK public API is subject to change.
//
// Set MALINA_LIB to the stable-diffusion.cpp library directory and
// MALINA_MODEL to an all-in-one checkpoint file before running:
//
//	MALINA_LIB=/path/to/libs MALINA_MODEL=/path/to/model.safetensors make example-malina
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

const (
	outputFile = "malina.png"
	prompt     = "a small red sailboat crossing a calm mountain lake at sunrise"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	modelPath := os.Getenv("MALINA_MODEL")
	if modelPath == "" {
		return errors.New("MALINA_MODEL is required")
	}

	if err := malina.Init(); err != nil {
		return fmt.Errorf("initialize Malina: %w", err)
	}

	m, err := malina.New(model.WithModelPath(modelPath))
	if err != nil {
		return fmt.Errorf("load model: %w", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fmt.Println("Unloading model")
		if err := m.Unload(ctx); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	info, err := malina.SystemInfo()
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}

	fmt.Println("Generating image")
	fmt.Println("- native version  :", info.NativeVersion)
	fmt.Println("- physical cores  :", info.PhysicalCores)
	fmt.Println("- backend devices :", info.BackendDeviceCount)
	fmt.Println("- model            :", modelPath)
	fmt.Println("- prompt           :", prompt)

	params := model.NewGenerateParams()
	params.Prompt = prompt
	params.Seed = 42

	start := time.Now()
	image, err := m.Generate(context.Background(), params)
	if err != nil {
		return fmt.Errorf("generate image: %w", err)
	}

	if err := os.WriteFile(outputFile, image.PNG, 0644); err != nil {
		return fmt.Errorf("write %s: %w", outputFile, err)
	}

	fmt.Println("- dimensions       :", fmt.Sprintf("%dx%d", image.Width, image.Height))
	fmt.Println("- seed             :", image.Seed)
	fmt.Println("- elapsed          :", time.Since(start).Round(time.Millisecond))
	fmt.Println("- output           :", outputFile)

	return nil
}
