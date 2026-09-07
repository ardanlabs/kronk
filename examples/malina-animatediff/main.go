// This example generates an AnimateDiff video and writes a Motion-JPEG AVI.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/malina/model"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	"github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func main() {
	if err := run(); err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()

	manifest, err := install(ctx)
	if err != nil {
		return err
	}

	mln, err := malina.New(
		model.WithModelPath(manifest.Files[string(models.RoleModel)]),
		model.WithMotionModulePath(manifest.Files[string(models.RoleMotionModule)]),
	)
	if err != nil {
		return err
	}

	defer func() {
		if err := mln.Unload(context.Background()); err != nil {
			fmt.Println("unload:", err)
		}
	}()

	params := model.NewVideoParams()
	params.Prompt = "a cat walking through a garden"

	video, err := mln.GenerateVideo(ctx, params)
	if err != nil {
		return err
	}

	return model.SaveAVI("malina-animatediff.avi", video.Frames, video.FPS, 90)
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

	return mdls.DownloadBundle(ctx, models.BundleAnimateDiffSD15)
}
