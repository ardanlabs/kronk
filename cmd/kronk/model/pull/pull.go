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

func runLocal(mdls *models.Models, basePath string, args []string) error {
	input := args[0]

	var projURL string
	if len(args) == 2 {
		projURL = args[1]
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	// Bare model ID or "provider/modelID": resolve via the new resolver.
	if isModelID(input) {
		rfile, err := defaults.CatalogFile("", basePath)
		if err != nil {
			return fmt.Errorf("resolver-file: %w", err)
		}

		r := models.NewResolver(mdls, rfile)

		res, err := r.Resolve(ctx, input)
		if err != nil {
			return fmt.Errorf("resolve: %w", err)
		}

		fmt.Printf("Resolved %s → %s/%s (%d file(s))\n", input, res.Provider, res.Family, len(res.DownloadURLs))

		downloadProj := res.DownloadProj
		if projURL != "" {
			downloadProj = projURL
		}

		if _, err := mdls.DownloadSplits(ctx, kronk.FmtLogger, res.DownloadURLs, downloadProj); err != nil {
			return fmt.Errorf("download-model: %w", err)
		}

		return nil
	}

	// Legacy: explicit URL or owner/repo/file.gguf path.
	if _, err := mdls.Download(ctx, kronk.FmtLogger, input, projURL); err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}

// isModelID reports whether the input looks like a bare model id or a
// "provider/modelID" pair, as opposed to a URL, an owner/repo/file.gguf
// path, or the legacy "owner/repo:TAG" shorthand.
func isModelID(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	// URLs are handled by the legacy path.
	if strings.Contains(s, "://") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(s), "huggingface.co/") || strings.HasPrefix(strings.ToLower(s), "hf.co/") {
		return false
	}

	// Legacy shorthand "owner/repo:TAG" (or with @revision).
	if strings.Contains(s, ":") {
		return false
	}

	// File paths and explicit gguf references.
	if strings.HasSuffix(strings.ToLower(s), ".gguf") {
		return false
	}

	// More than one "/" indicates a path (owner/repo/file.gguf style).
	if strings.Count(s, "/") > 1 {
		return false
	}

	return true
}
