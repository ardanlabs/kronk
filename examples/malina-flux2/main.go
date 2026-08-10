// This example shows you how to generate an image with the Malina SDK and a
// multi-file FLUX.2 model.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries and the FLUX.2 Klein 9B model bundle.
//
// Run the example like this from the root of the project:
// $ make example-malina-flux2

package main

import (
	"context"
	"fmt"
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
	modelSource = models.BundleFlux2Klein9B
	progressMu  sync.Mutex
)

const outputFile = "malina-flux2.png"

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	manifest, err := installSystem()
	if err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	mln, err := newMalina(manifest)
	if err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}
	defer func() {
		fmt.Println("\nUnloading Malina")
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Printf("unload: %v\n", err)
		}
	}()

	if err := generate(mln); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	return nil
}

// =============================================================================

func installSystem() (models.Manifest, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
	)
	if err != nil {
		return models.Manifest{}, err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(
		malina.WithLibPath(libs.LibsPath()),
		malina.WithProgress(progress),
	); err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init Malina: %w", err)
	}

	// -------------------------------------------------------------------------

	mdls, err := models.New()
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to init models: %w", err)
	}

	fmt.Println("Downloading model bundle:", modelSource)

	manifest, err := mdls.DownloadBundle(ctx, modelSource)
	if err != nil {
		return models.Manifest{}, fmt.Errorf("unable to install model bundle: %w", err)
	}

	return manifest, nil
}

func newMalina(manifest models.Manifest) (*malina.Malina, error) {
	fmt.Println("Loading model...")

	mln, err := malina.New(
		model.WithDiffusionModelPath(manifest.Files[string(models.RoleDiffusion)]),
		model.WithVAEPath(manifest.Files[string(models.RoleVAE)]),
		model.WithLLMPath(manifest.Files[string(models.RoleLLM)]),
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
	fmt.Println("- model             :", mi.DiffusionModelPath)
	fmt.Println("- cpu threads       :", cfg.CPUThreads)
	fmt.Println("- active generations:", mln.ActiveGenerations())

	return mln, nil
}

// =============================================================================

func generate(mln *malina.Malina) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	params := model.NewGenerateParams()
	params.Prompt = "an orange cat on a tropical beach playing with oranges"
	params.NegativePrompt = "mascots, watermark, signature"
	params.Steps = 4

	fmt.Println("\nGenerating FLUX.2 image...")
	start := time.Now()

	image, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputFile, image.PNG, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", outputFile, err)
	}

	fmt.Printf("Wrote %s (%dx%d) in %s\n", outputFile, image.Width, image.Height, time.Since(start).Round(time.Millisecond))

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
