// This example transforms an existing image with the Malina SDK.
//
// Experimental: The Malina SDK public API is subject to change.
//
// Set MALINA_LIB and MALINA_MODEL, then run with a PNG or JPEG source:
//
//	MALINA_LIB=/path/to/libs MALINA_MODEL=/path/to/model.safetensors \
//	make example-malina-img2img
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
)

type config struct {
	input    string
	output   string
	prompt   string
	strength float64
	steps    int
	seed     int64
}

func main() {
	var cfg config
	flag.StringVar(&cfg.input, "in", "samples/giraffe.jpg", "source PNG or JPEG path")
	flag.StringVar(&cfg.output, "out", "malina-img2img.png", "output PNG path")
	flag.StringVar(&cfg.prompt, "prompt", "a watercolor painting at sunset", "prompt that steers the image")
	flag.Float64Var(&cfg.strength, "strength", 0.6, "noise strength in (0,1]")
	flag.IntVar(&cfg.steps, "steps", 20, "denoising steps")
	flag.Int64Var(&cfg.seed, "seed", 42, "RNG seed (-1 selects a random seed)")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	modelPath := os.Getenv("MALINA_MODEL")
	if modelPath == "" {
		return errors.New("MALINA_MODEL is required")
	}

	source, err := loadImage(cfg.input)
	if err != nil {
		return err
	}

	if err := malina.Init(); err != nil {
		return fmt.Errorf("initialize Malina: %w", err)
	}

	m, err := malina.New(model.WithModelPath(modelPath))
	if err != nil {
		return fmt.Errorf("load img2img model: %w", err)
	}
	defer unload(m)

	params := model.NewGenerateParams()
	params.Prompt = cfg.prompt
	params.InitImage = source
	params.Strength = float32(cfg.strength)
	params.Steps = cfg.steps
	params.Seed = cfg.seed
	params.Width, params.Height, err = generationSize(source.Bounds())
	if err != nil {
		return err
	}

	fmt.Printf("Transforming %s with strength %.2f\n", cfg.input, cfg.strength)
	start := time.Now()
	generated, err := m.Generate(context.Background(), params)
	if err != nil {
		return fmt.Errorf("generate image: %w", err)
	}
	if err := os.WriteFile(cfg.output, generated.PNG, 0644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.output, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\n", cfg.output, generated.Width, generated.Height, time.Since(start).Round(time.Millisecond))

	return nil
}

func loadImage(filename string) (image.Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read source image: %w", err)
	}

	image, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode source image: %w", err)
	}

	return image, nil
}

func generationSize(bounds image.Rectangle) (int, int, error) {
	const (
		alignment    = 8
		minDimension = 64
		maxDimension = 1024
	)

	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return 0, 0, errors.New("source image dimensions must be positive")
	}

	scale := min(1, min(float64(maxDimension)/float64(width), float64(maxDimension)/float64(height)))
	width = int(float64(width)*scale) / alignment * alignment
	height = int(float64(height)*scale) / alignment * alignment
	if width < minDimension || height < minDimension {
		return 0, 0, fmt.Errorf("source aspect ratio produces dimensions below %d pixels", minDimension)
	}

	return width, height, nil
}

func unload(m *malina.Malina) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Unload(ctx); err != nil {
		fmt.Printf("unload: %v\n", err)
	}
}
