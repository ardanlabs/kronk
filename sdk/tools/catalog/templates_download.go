package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func (t *templates) download(ctx context.Context, log func(context.Context, string, ...any)) error {
	if !hasNetwork() {
		log(ctx, "template-download", "status", "no network available")
		return nil
	}

	log(ctx, "template-download", "status", "retrieving template files", "github", t.githubRepo)

	files, err := t.templateListGitHubFolder(ctx)
	if err != nil {
		return fmt.Errorf("download: listing templates: %w", err)
	}

	if len(files) > 0 {
		log(ctx, "template-download", "status", "download template changes")

		for _, file := range files {
			if err := t.downloadTemplateFile(ctx, file); err != nil {
				return fmt.Errorf("download-template: %w", err)
			}
		}
	}

	return nil
}

func (t *templates) downloadTemplateFile(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("download-file: creating request: %w", err)
	}

	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("If-None-Match", "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download-file: fetching file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download-file: unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("download-file: reading response: %w", err)
	}

	filePath := filepath.Join(t.templatePath, filepath.Base(url))
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return fmt.Errorf("download-file: writing file: %w", err)
	}

	return nil
}

// =============================================================================

func (t *templates) templateListGitHubFolder(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.githubRepo, nil)
	if err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: creating request: %w", err)
	}

	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("If-None-Match", "")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: fetching folder listing: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list-git-hub-folder: unexpected status: %s", resp.Status)
	}

	var items []gitHubFile
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	localSHAs := t.readTemplateSHAs()

	var files []string
	for _, item := range items {
		if item.Type != "file" || item.DownloadURL == "" {
			continue
		}
		if localSHAs[item.Name] != item.SHA {
			files = append(files, item.DownloadURL)
		}
	}

	if err := t.writeTemplateSHAs(items); err != nil {
		return nil, fmt.Errorf("list-git-hub-folder: writing SHA file: %w", err)
	}

	return files, nil
}

func (t *templates) readTemplateSHAs() map[string]string {
	data, err := os.ReadFile(filepath.Join(t.templatePath, templateSHAFile))
	if err != nil {
		return make(map[string]string)
	}

	var shas map[string]string
	if err := json.Unmarshal(data, &shas); err != nil {
		return make(map[string]string)
	}

	return shas
}

func (t *templates) writeTemplateSHAs(items []gitHubFile) error {
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

	return os.WriteFile(filepath.Join(t.templatePath, templateSHAFile), data, 0644)
}
