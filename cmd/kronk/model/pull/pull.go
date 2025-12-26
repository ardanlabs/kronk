// Package pull provides the pull command code.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	url, err := client.DefaultURL("/v1/models/pull")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	var modelProj string
	if len(args) == 2 {
		modelProj = args[1]
	}

	body := client.D{
		"model_url": args[0],
		"proj_url":  modelProj,
	}

	cln := client.NewSSE[toolapp.PullResponse](
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ch := make(chan toolapp.PullResponse)
	if err := cln.Do(ctx, http.MethodPost, url, body, ch); err != nil {
		return fmt.Errorf("do: unable to download model: %w", err)
	}

	for ver := range ch {
		fmt.Print(ver.Status)
	}

	fmt.Println()

	return nil
}

func runLocal(models *models.Models, args []string) error {
	modelURL := args[0]

	var projURL string
	if len(args) == 2 {
		projURL = args[1]
	}

	if _, err := url.ParseRequestURI(modelURL); err != nil {
		return fmt.Errorf("parse-request-uri: invalid URL: %s", modelURL)
	}

	if projURL != "" {
		if _, err := url.ParseRequestURI(projURL); err != nil {
			return fmt.Errorf("parse-request-uri: invalid project URL: %s", projURL)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	_, err := models.Download(ctx, kronk.FmtLogger, modelURL, projURL)
	if err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}
