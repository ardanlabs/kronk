// This example enlarges a PNG or JPEG using Real-ESRGAN.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	manifest, err := install(ctx)
	if err != nil {
		return err
	}

	input, err := loadImage(imageFile)
	if err != nil {
		return err
	}

	upscaler, err := malina.NewUpscaler(ctx, malina.UpscalerConfig{ModelPath: manifest.Files[string(models.RoleUpscaler)]})
	if err != nil {
		return err
	}

	defer func() {
		if err := upscaler.Unload(); err != nil {
			fmt.Println("unload:", err)
		}
	}()

	images, err := upscaler.Upscale(ctx, input)
	if err != nil {
		return err
	}

	if len(images) == 0 {
		return fmt.Errorf("upscaler returned no images")
	}

	file, err := os.Create("malina-upscaled.png")
	if err != nil {
		return err
	}

	return errors.Join(png.Encode(file, images[0]), file.Close())
}

func install(ctx context.Context) (models.Manifest, error) {
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

	return mdls.DownloadBundle(ctx, models.BundleRealESRGANX4Anime)
}

func loadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	img, _, err := image.Decode(file)
	return img, errors.Join(err, file.Close())
}
