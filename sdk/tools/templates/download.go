package templates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Logger represents a logger for capturing events.
type Logger func(ctx context.Context, msg string, args ...any)

type options struct {
	log Logger
}

// Option represents a functional option for configuring Kronk.
type Option func(*options)

// WithLogger sets a logger for the download call.
func WithLogger(log Logger) Option {
	return func(o *options) {
		o.log = log
	}
}

// Download retrieves the templates from the github repo. Only files modified
// after the last download are fetched.
func (t *Templates) Download(ctx context.Context, opts ...Option) error {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	log := func(ctx context.Context, msg string, args ...any) {
		if o.log != nil {
			o.log(ctx, msg, args...)
		}
	}

	if !hasNetwork() {
		log(ctx, "template-download", "status", "no network avaialble")
		return nil
	}

	log(ctx, "template-download", "status", "retrieving template files", "github", t.githubRepoPath)

	files, err := t.listGitHubFolder(ctx)
	if err != nil {
		return fmt.Errorf("listing templates: %w", err)
	}

	if len(files) > 0 {
		log(ctx, "template-download", "status", "download template changes")

		for _, file := range files {
			if err := t.downloadFile(ctx, file); err != nil {
				return fmt.Errorf("download-template: %w", err)
			}
		}
	}

	return nil
}

func (t *Templates) downloadFile(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	filePath := filepath.Join(t.templatePath, filepath.Base(url))
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	return nil
}

// =============================================================================

type gitHubFile struct {
	Name        string `json:"name"`
	SHA         string `json:"sha"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
}

func (t *Templates) listGitHubFolder(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.githubRepoPath, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching folder listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var items []gitHubFile
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	localSHAs := t.readLocalSHAs()

	var files []string
	for _, item := range items {
		if item.Type != "file" || item.DownloadURL == "" {
			continue
		}
		if localSHAs[item.Name] != item.SHA {
			files = append(files, item.DownloadURL)
		}
	}

	if err := t.writeLocalSHAs(items); err != nil {
		return nil, fmt.Errorf("writing SHA file: %w", err)
	}

	return files, nil
}

func (t *Templates) readLocalSHAs() map[string]string {
	data, err := os.ReadFile(filepath.Join(t.templatePath, shaFile))
	if err != nil {
		return make(map[string]string)
	}

	var shas map[string]string
	if err := json.Unmarshal(data, &shas); err != nil {
		return make(map[string]string)
	}

	return shas
}

func (t *Templates) writeLocalSHAs(items []gitHubFile) error {
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

	return os.WriteFile(filepath.Join(t.templatePath, shaFile), data, 0644)
}

// =============================================================================

func hasNetwork() bool {
	conn, err := net.DialTimeout("tcp", "8.8.8.8:53", 3*time.Second)
	if err != nil {
		return false
	}

	conn.Close()

	return true
}
