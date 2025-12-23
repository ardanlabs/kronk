// Package catalog provides tooling support for the catalog system.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	defaultGithubPath = "https://api.github.com/repos/ardanlabs/kronk_catalogs/contents/catalogs"
	localFolder       = "catalogs"
	indexFile         = ".index.yaml"
)

// Catalog manages the catalog system.
type Catalog struct {
	catalogDir     string
	githubRepoPath string
	biMutex        sync.Mutex
}

// New constructs the catalog system, using the specified github
// repo path. If the path is empty, the default repo is used.
func New(basePath string, githubRepoPath string) (*Catalog, error) {
	if githubRepoPath == "" {
		githubRepoPath = defaultGithubPath
	}

	catalogDir := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return nil, fmt.Errorf("creating catalogs directory: %w", err)
	}

	c := Catalog{
		catalogDir:     catalogDir,
		githubRepoPath: githubRepoPath,
	}

	return &c, nil
}
