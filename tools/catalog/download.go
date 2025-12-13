package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.yaml.in/yaml/v2"
)

var files = []string{
	"https://raw.githubusercontent.com/ardanlabs/kronk_catalogs/refs/heads/main/catalogs/audio_text_to_text.yaml",
	"https://raw.githubusercontent.com/ardanlabs/kronk_catalogs/refs/heads/main/catalogs/embedding.yaml",
	"https://raw.githubusercontent.com/ardanlabs/kronk_catalogs/refs/heads/main/catalogs/image_text_to_text.yaml",
	"https://raw.githubusercontent.com/ardanlabs/kronk_catalogs/refs/heads/main/catalogs/text_generation.yaml",
}

// Download retrieves the catalog from github.com/ardanlabs/kronk_catalogs.
func Download(ctx context.Context, basePath string) error {
	for _, file := range files {
		if err := downloadCatalog(ctx, basePath, file); err != nil {
			return fmt.Errorf("download-catalog: %w", err)
		}
	}

	if err := buildIndex(basePath); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	return nil
}

// =============================================================================

func downloadCatalog(ctx context.Context, basePath string, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	catalogDir := filepath.Join(basePath, localFolder)
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return fmt.Errorf("creating catalogs directory: %w", err)
	}

	filePath := filepath.Join(catalogDir, filepath.Base(url))
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		return fmt.Errorf("writing catalog file: %w", err)
	}

	return nil
}

var biMutex sync.Mutex

func buildIndex(basePath string) error {
	biMutex.Lock()
	defer biMutex.Unlock()

	catalogDir := filepath.Join(basePath, localFolder)

	entries, err := os.ReadDir(catalogDir)
	if err != nil {
		return fmt.Errorf("read catalog dir: %w", err)
	}

	index := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}

		if entry.Name() == indexFile {
			continue
		}

		filePath := filepath.Join(catalogDir, entry.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read file %s: %w", entry.Name(), err)
		}

		var catalog Catalog
		if err := yaml.Unmarshal(data, &catalog); err != nil {
			return fmt.Errorf("unmarshal %s: %w", entry.Name(), err)
		}

		for _, model := range catalog.Models {
			modelID := strings.ToLower(model.ID)
			index[modelID] = entry.Name()
		}
	}

	indexData, err := yaml.Marshal(&index)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}

	indexPath := filepath.Join(catalogDir, indexFile)
	if err := os.WriteFile(indexPath, indexData, 0644); err != nil {
		return fmt.Errorf("write index file: %w", err)
	}

	return nil
}
