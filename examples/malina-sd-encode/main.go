// This example shows you how to encode PNG and JPEG frames into a Motion-JPEG
// AVI with the Malina SDK.
//
// Experimental: The Malina SDK public API is subject to change.
//
// No model or native library is required.
//
// Run the example like this from the root of the project:
// $ make example-malina-sd-encode

package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/malina/model"
	"golang.org/x/image/draw"
)

type config struct {
	inputDir string
	output   string
	fps      int
	quality  int
}

func main() {
	var cfg config
	flag.StringVar(&cfg.inputDir, "i", "samples/deer", "directory containing PNG and JPEG frames")
	flag.StringVar(&cfg.output, "o", "malina-output.avi", "output AVI path")
	flag.IntVar(&cfg.fps, "fps", 24, "frames per second")
	flag.IntVar(&cfg.quality, "quality", 90, "JPEG quality from 1 to 100")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run(cfg config) error {
	paths, err := imagePaths(cfg.inputDir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no PNG or JPEG files found in %s", cfg.inputDir)
	}

	frames := make([]image.Image, 0, len(paths))
	var target image.Rectangle
	for _, path := range paths {
		frame, err := loadImage(path)
		if err != nil {
			return err
		}
		if target.Empty() {
			target = image.Rect(0, 0, frame.Bounds().Dx(), frame.Bounds().Dy())
		}
		frames = append(frames, resize(frame, target))
	}

	if err := model.SaveAVI(cfg.output, frames, cfg.fps, cfg.quality); err != nil {
		return fmt.Errorf("save AVI: %w", err)
	}

	fmt.Printf("Wrote %s (%d frames, %dx%d at %d fps)\n", cfg.output, len(frames), target.Dx(), target.Dy(), cfg.fps)

	return nil
}

func imagePaths(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read frames directory: %w", err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".jpg", ".jpeg", ".png":
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	slices.Sort(paths)

	return paths, nil
}

func loadImage(filename string) (image.Image, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", filename, err)
	}

	frame, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", filename, err)
	}

	return frame, nil
}

func resize(source image.Image, target image.Rectangle) image.Image {
	if source.Bounds().Dx() == target.Dx() && source.Bounds().Dy() == target.Dy() {
		return source
	}

	frame := image.NewRGBA(target)
	draw.BiLinear.Scale(frame, target, source, source.Bounds(), draw.Src, nil)

	return frame
}
