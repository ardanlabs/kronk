// Package pull provides the pull command code.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(source string, projURL string) error {
	url, err := client.DefaultURL("/v1/models/pull")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	body := client.D{
		"model_url": source,
		"proj_url":  projURL,
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

func runLocal(mdls *models.Models, basePath string, source string, projURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Default workflow — Download handles every input form (bare id,
	// canonical id, full URL, owner/repo/file.gguf path) and locates
	// the projection file automatically.
	if projURL == "" {
		if _, err := mdls.Download(ctx, kronk.FmtLogger, source); err != nil {
			return fmt.Errorf("download-model: %w", err)
		}

		return nil
	}

	// Explicit projection override — full-control workflow. When the
	// source is a URL, pair it directly with the supplied projection
	// URL. When the source is an id, the resolver is consulted only to
	// expand split (multi-file) models; the supplied projection URL
	// replaces the resolver's choice.
	if isURL(source) {
		if _, err := mdls.DownloadURLs(ctx, kronk.FmtLogger, []string{source}, projURL); err != nil {
			return fmt.Errorf("download-model: %w", err)
		}

		return nil
	}

	rfile, err := defaults.CatalogFile("", basePath)
	if err != nil {
		return fmt.Errorf("resolver-file: %w", err)
	}

	res, err := models.NewResolver(mdls, rfile).Resolve(ctx, source)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	fmt.Printf("Resolved %s → %s/%s (%d file(s))\n", source, res.Provider, res.Family, len(res.DownloadURLs))

	if _, err := mdls.DownloadURLs(ctx, kronk.FmtLogger, res.DownloadURLs, projURL); err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}

// isURL reports whether the source is a full HTTP(S) URL.
func isURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}
