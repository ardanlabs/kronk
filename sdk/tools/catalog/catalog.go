// Package catalog provides tooling support for the catalog system.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ardanlabs/kronk/sdk/tools/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	defaultGithubPath = "https://api.github.com/repos/ardanlabs/kronk_catalogs/contents/catalogs"
	localFolder       = "catalogs"
	indexFile         = ".index.yaml"
)

// Catalog manages the catalog system.
type Catalog struct {
	catalogPath    string
	githubRepoPath string
	models         *models.Models
	biMutex        sync.Mutex
}

// New constructs the catalog system using defaults paths.
func New() (*Catalog, error) {
	return NewWithSettings("", "")
}

// NewWithSettings constructs the catalog system, using the specified github
// repo path. If either path is empty, the default paths are used.
func NewWithSettings(basePath string, githubRepoPath string) (*Catalog, error) {
	basePath = defaults.BaseDir(basePath)

	if githubRepoPath == "" {
		githubRepoPath = defaultGithubPath
	}

	catalogPath := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(catalogPath, 0755); err != nil {
		return nil, fmt.Errorf("creating catalogs directory: %w", err)
	}

	models, err := models.New()
	if err != nil {
		return nil, fmt.Errorf("creating models system: %w", err)
	}

	c := Catalog{
		catalogPath:    catalogPath,
		githubRepoPath: githubRepoPath,
		models:         models,
	}

	return &c, nil
}

// CatalogPath returns the location of the catalog path.
func (c *Catalog) CatalogPath() string {
	return c.catalogPath
}
