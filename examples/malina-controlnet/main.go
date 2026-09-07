// This example generates an image using Canny ControlNet conditioning.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

const imageFile = "samples/adetailer-face.png"

func main() {
	if err := run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	manifest, err := install(ctx, models.BundleControlNetCannySD15)
	if err != nil {
		return err
	}

	input, err := loadImage(imageFile)
	if err != nil {
		return err
	}

	mln, err := malina.New(
		model.WithModelPath(manifest.Files[string(models.RoleModel)]),
		model.WithControlNetPath(manifest.Files[string(models.RoleControlNet)]),
	)
	if err != nil {
		return err
	}

	defer func() {
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Println("unload:", err)
		}
	}()

	params := model.NewGenerateParams()
	params.Prompt = "a detailed astronaut portrait"
	params.Width = input.Bounds().Dx()
	params.Height = input.Bounds().Dy()
	params.ControlImage = input
	canny := model.NewCannyParams()
	params.Canny = &canny

	result, err := mln.Generate(ctx, params)
	if err != nil {
		return err
	}

	return os.WriteFile("malina-controlnet.png", result.PNG, 0o644)
}

func install(ctx context.Context, bundle models.BundleName) (models.Manifest, error) {
	lib, err := libs.New(libs.WithDetect(ctx, malina.FmtLogger), libs.WithValidation(true))
	if err != nil {
		return models.Manifest{}, err
	}

	if _, err := lib.Download(ctx, malina.FmtLogger); err != nil {
		return models.Manifest{}, err
	}

	if err := malina.Init(malina.WithLibPath(lib.LibsPath())); err != nil {
		return models.Manifest{}, err
	}

	mdls, err := models.New()
	if err != nil {
		return models.Manifest{}, err
	}

	return mdls.DownloadBundle(ctx, bundle)
}

func loadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(file)
	return img, errors.Join(err, file.Close())
}
