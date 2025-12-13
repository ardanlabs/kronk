// Package pull provides the pull command code.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// RunWeb executes the libs command against the model server.
func RunWeb(args []string) error {
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

	cln := client.NewSSE[toolapp.PullResponse](client.FmtLogger)

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

// RunLocal executes the pull command.
func RunLocal(args []string) error {
	modelBasePath := defaults.ModelsDir("")
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	_, err := tools.DownloadModel(ctx, kronk.FmtLogger, modelURL, projURL, modelBasePath)
	if err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}
