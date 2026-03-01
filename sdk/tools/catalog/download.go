package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/github"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	shaFile = ".catalog_shas.json"
)

// Logger represents a logger for capturing events.
type Logger func(ctx context.Context, msg string, args ...any)

type downloadOptions struct {
	log Logger
}

// DownloadOption represents option for the download.
type DownloadOption func(*downloadOptions)

// WithLogger sets a logger for the download call.
func WithLogger(log Logger) DownloadOption {
	return func(o *downloadOptions) {
		o.log = log
	}
}

// Download retrieves the catalog from the github repo. Only files modified
// after the last download are fetched.
func (c *Catalog) Download(ctx context.Context, opts ...DownloadOption) error {
	var o downloadOptions
	for _, opt := range opts {
		opt(&o)
	}

	log := func(ctx context.Context, msg string, args ...any) {
		if o.log != nil {
			o.log(ctx, msg, args...)
		}
	}

	if !hasNetwork() {
		log(ctx, "catalog-download", "status", "no network available")
		return nil
	}

	log(ctx, "catalog-download", "status", "retrieving catalog files", "github", c.githubRepo)

	files, err := c.listGitHubFolder(ctx)
	if err != nil {
		if errors.Is(err, github.ErrRateLimited) {
			logRateLimit(ctx, log, "catalog-download", c.ghClient)
		}
		log(ctx, "catalog-download", "WARNING", "unable to retrieve catalog files, using local cache", "error", err.Error())
	}

	if len(files) > 0 {
		log(ctx, "catalog-download", "status", "download catalog changes")

		for _, file := range files {
			if err := c.downloadCatalog(ctx, file); err != nil {
				if errors.Is(err, github.ErrRateLimited) {
					logRateLimit(ctx, log, "catalog-download", c.ghClient)
					log(ctx, "catalog-download", "WARNING", "github rate limited, using local cache", "error", err.Error())
					break
				}
				return fmt.Errorf("download-catalog: %w", err)
			}
		}

		log(ctx, "catalog-download", "status", "building index")
	}

	if err := c.buildIndex(); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	if err := c.templates.download(ctx, log); err != nil {
		return fmt.Errorf("download-templates: %w", err)
	}

	if err := c.grammars.download(ctx, log); err != nil {
		return fmt.Errorf("download-grammars: %w", err)
	}

	return nil
}

// DownloadModel downloads the specified model from the catalog system.
func (c *Catalog) DownloadModel(ctx context.Context, log Logger, modelID string) (models.Path, error) {
	model, err := c.Details(modelID)
	if err != nil {
		return models.Path{}, fmt.Errorf("retrieve-model-details: %w", err)
	}

	return c.models.DownloadSplits(ctx, models.Logger(log), model.Files.ToModelURLS(), model.Files.Proj.URL)
}

// =============================================================================

type gitHubFile struct {
	Name        string `json:"name"`
	SHA         string `json:"sha"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
}

func (c *Catalog) listGitHubFolder(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.githubRepo, nil)
	if err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("If-None-Match", "")

	resp, err := c.ghClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: fetching folder listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list-git-hub-folder: unexpected status: %s", resp.Status)
	}

	var items []gitHubFile
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: decoding response: %w", err)
	}

	localSHAs := c.readLocalSHAs()

	var files []string
	for _, item := range items {
		if item.Type != "file" || item.DownloadURL == "" {
			continue
		}
		if localSHAs[item.Name] != item.SHA {
			files = append(files, item.DownloadURL)
		}
	}

	if err := c.writeLocalSHAs(items); err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: writing SHA file: %w", err)
	}

	return files, nil
}

func (c *Catalog) downloadCatalog(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download-catalog: creating request: %w", err)
	}

	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("If-None-Match", "")

	resp, err := c.ghClient.Do(req)
	if err != nil {
		return fmt.Errorf("download-catalog: fetching catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download-catalog: unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("download-catalog: reading response: %w", err)
	}

	filePath := filepath.Join(c.catalogPath, filepath.Base(url))
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return fmt.Errorf("download-catalog: writing catalog file: %w", err)
	}

	return nil
}

func (c *Catalog) readLocalSHAs() map[string]string {
	data, err := os.ReadFile(filepath.Join(c.catalogPath, shaFile))
	if err != nil {
		return make(map[string]string)
	}

	var shas map[string]string
	if err := json.Unmarshal(data, &shas); err != nil {
		return make(map[string]string)
	}

	return shas
}

func (c *Catalog) writeLocalSHAs(items []gitHubFile) error {
	shas := make(map[string]string)
	for _, item := range items {
		if item.Type == "file" {
			shas[item.Name] = item.SHA
		}
	}

	data, err := json.Marshal(shas)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(c.catalogPath, shaFile), data, 0644)
}

func logRateLimit(ctx context.Context, log func(context.Context, string, ...any), caller string, ghClient *github.Client) {
	rl := ghClient.RateLimitState()

	log(ctx, caller,
		"WARNING", "github rate limit details",
		"limit", rl.Limit,
		"remaining", rl.Remaining,
		"used", rl.Used,
		"reset", rl.Reset.Format(time.RFC3339),
		"resource", rl.Resource,
	)
}

// =============================================================================

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 5*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
