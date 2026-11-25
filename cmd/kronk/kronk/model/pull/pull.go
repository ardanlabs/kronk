// Package pull provides the pull command code.
package pull

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/status"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(ctx context.Context, source string, projURL string, draft models.CatalogDraft) error {
	url, err := client.DefaultURL("/v1/kronk/models/pull")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	body := client.D{
		"model_url": source,
		"proj_url":  projURL,
	}
	if draft != (models.CatalogDraft{}) {
		body["draft"] = draft
	}

	cln := client.NewSSE[toolapp.PullResponse](
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ch := make(chan toolapp.PullResponse)
	errCh, err := cln.DoWithErrors(ctx, http.MethodPost, url, body, ch)
	if err != nil {
		return fmt.Errorf("do: unable to download model: %w", err)
	}

	prt := progressPrinter{
		w:       os.Stdout,
		rewrite: isTerminal(os.Stdout),
	}

	var lastStatus string
	var downloaded bool
	for ver := range ch {
		prt.print(ver)
		if ver.Progress != nil {
			status.Download(ctx, ver.Progress.Src, ver.Progress.CurrentBytes, ver.Progress.TotalBytes)
		}
		lastStatus = ver.Status
		downloaded = downloaded || ver.Downloaded || ver.Status == "downloaded"
	}
	prt.close()

	if err := <-errCh; err != nil {
		return fmt.Errorf("download stream: %w", err)
	}

	if !downloaded {
		if lastStatus == "" {
			lastStatus = "stream ended before completion"
		}
		return fmt.Errorf("download-model: %s", lastStatus)
	}

	return nil
}

func runLocal(ctx context.Context, mdls *models.Models, source string, projURL string, draft models.CatalogDraft) error {
	prt := progressPrinter{
		w:       os.Stdout,
		rewrite: isTerminal(os.Stdout),
	}
	defer prt.close()

	log := func(ctx context.Context, msg string, args ...any) {
		prt.close()
		kronk.FmtLogger(ctx, msg, args...)
	}

	progress := func(progress models.DownloadProgress) {
		status.Download(ctx, progress.Src, progress.CurrentBytes, progress.TotalBytes)
		msg := fmt.Sprintf("download-model: Downloading %s... %d MB of %d MB (%.2f MB/s)",
			progress.Src,
			progress.CurrentBytes/(1000*1000),
			progress.TotalBytes/(1000*1000),
			progress.MBPerSec,
		)
		prt.progress(msg, progress.Src, progress.Complete)
	}

	// Default workflow — Download handles every input form (canonical id,
	// full URL, owner/repo/file.gguf path) and locates
	// both the projection and draft companions automatically. This
	// mirrors the model server's pull endpoint (and the BUI).
	if projURL == "" && draft.URL == "" {
		if _, err := mdls.DownloadWithProgress(ctx, log, source, progress); err != nil {
			return fmt.Errorf("download-model: %w", err)
		}

		return nil
	}

	// Explicit companion override — full-control workflow. When the
	// source is a URL, pair it directly with the supplied companion URLs.
	if isURL(source) {
		if _, err := mdls.DownloadDraftURLsWithProgress(ctx, log, []string{source}, projURL, draft, progress); err != nil {
			return fmt.Errorf("download-model: %w", err)
		}

		return nil
	}

	// Id source: the resolver expands split (multi-file) models. A
	// companion the caller pinned replaces the resolver's choice; a
	// companion left empty is auto-resolved so the draft companion (and
	// projection) is always fetched when the catalog knows about it.
	res, err := mdls.ResolveSource(ctx, source)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	if projURL == "" {
		projURL = res.DownloadProj
	}
	if draft.URL == "" {
		draft = res.Draft
		draft.URL = res.DownloadDraft
	}

	fmt.Printf("Resolved %s → %s/%s (%d file(s))\n", source, res.Provider, res.Family, len(res.DownloadURLs))

	if _, err := mdls.DownloadDraftURLsWithProgress(ctx, log, res.DownloadURLs, projURL, draft, progress); err != nil {
		return fmt.Errorf("download-model: %w", err)
	}

	return nil
}

// isURL reports whether the source is a full HTTP(S) URL.
func isURL(source string) bool {
	return strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://")
}
