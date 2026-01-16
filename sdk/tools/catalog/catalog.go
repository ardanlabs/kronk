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

// =============================================================================

type options struct {
	basePath   string
	githubRepo string
}

// Option represents options for configuring catalog.
type Option func(*options)

// WithBasePath sets a custom base path on disk for the templates.
func WithBasePath(basePath string) Option {
	return func(o *options) {
		o.basePath = basePath
	}
}

// WithGithubRepo sets a custom github repo url.
func WithGithubRepo(githubRepo string) Option {
	return func(o *options) {
		o.githubRepo = githubRepo
	}
}

// =============================================================================

// Catalog manages the catalog system.
type Catalog struct {
	catalogPath string
	githubRepo  string
	models      *models.Models
	biMutex     sync.Mutex
}

// New constructs the catalog system using defaults paths.
func New(opts ...Option) (*Catalog, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	o.basePath = defaults.BaseDir(o.basePath)

	if o.githubRepo == "" {
		o.githubRepo = defaultGithubPath
	}

	catalogPath := filepath.Join(o.basePath, localFolder)

	if err := os.MkdirAll(catalogPath, 0755); err != nil {
		return nil, fmt.Errorf("new: creating catalogs directory: %w", err)
	}

	models, err := models.NewWithPaths(o.basePath)
	if err != nil {
		return nil, fmt.Errorf("new: creating models system: %w", err)
	}

	c := Catalog{
		catalogPath: catalogPath,
		githubRepo:  o.githubRepo,
		models:      models,
	}

	return &c, nil
}

// CatalogPath returns the location of the catalog path.
func (c *Catalog) CatalogPath() string {
	return c.catalogPath
}
