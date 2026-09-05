// This example shows you how to transform an existing image with the Malina
// SDK.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and a Stable Diffusion 1.5 model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina-img2img

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
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

var (
	modelSource = models.BundleSD15.String()
	progressMu  sync.Mutex
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
	mp, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	mln, err := newMalina(mp)
	if err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Malina")
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	if err := transform(mln, cfg); err != nil {
		return fmt.Errorf("transform: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (models.Path, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return models.Path{}, err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return models.Path{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(
		malina.WithLibPath(libs.LibsPath()),
		malina.WithProgress(progress),
	); err != nil {
		return models.Path{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	mp, err := mdls.Download(ctx, malina.FmtLogger, modelSource)
	if err != nil {
		return models.Path{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return mp, nil
}

func newMalina(mp models.Path) (*malina.Malina, error) {
	fmt.Println("Loading model...")

	if len(mp.ModelFiles) == 0 {
		return nil, fmt.Errorf("no model files on disk")
	}

	mln, err := malina.New(
		model.WithModelPath(mp.ModelFiles[0]),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to create image generation model: %w", err)
	}

	si := mln.SystemInfo()
	cfg := mln.ModelConfig()
	mi := mln.ModelInfo()

	fmt.Println("- native version    :", si.NativeVersion)
	fmt.Println("- physical cores    :", si.PhysicalCores)
	fmt.Println("- backend devices   :", si.BackendDeviceCount)
	fmt.Println("- model             :", mi.ModelPath)
	fmt.Println("- cpu threads       :", cfg.CPUThreads)
	fmt.Println("- concurrency       :", cfg.Concurrency)
	fmt.Println("- queue depth       :", cfg.QueueDepth)
	fmt.Println("- active generations:", mln.ActiveGenerations())

	return mln, nil
}

// =============================================================================

func transform(mln *malina.Malina, cfg config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	source, err := loadImage(cfg.input)
	if err != nil {
		return err
	}

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

	fmt.Printf("\nTransforming %s with strength %.2f...\n", cfg.input, cfg.strength)
	start := time.Now()

	generated, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfg.output, generated.PNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", cfg.output, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\n", cfg.output, generated.Width, generated.Height, time.Since(start).Round(time.Millisecond))

	return nil
}

// progress renders model loading and image generation progress reported by
// stable-diffusion.cpp.
func progress(step int, steps int, secondsPerStep float32) {
	if step <= 0 || steps <= 0 {
		return
	}

	progressMu.Lock()
	defer progressMu.Unlock()

	const width = 50
	current := min(step, steps)
	filled := min(current*width/steps, width)
	bar := strings.Repeat("=", filled)
	if filled < width {
		bar += ">"
	}

	speed := fmt.Sprintf("%.2fs/it", secondsPerStep)
	if secondsPerStep > 0 && secondsPerStep < 1 {
		speed = fmt.Sprintf("%.2fit/s", 1/secondsPerStep)
	}

	fmt.Printf("\r  |%-50s| %d/%d - %s\x1b[K", bar, current, steps, speed)
	if current == steps {
		fmt.Println()
	}
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
