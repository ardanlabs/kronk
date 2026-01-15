// Package templates provides template support.
package templates

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/defaults"
)

const (
	defaultGithubPath = "https://api.github.com/repos/ardanlabs/kronk_catalogs/contents/templates"
	localFolder       = "templates"
	shaFile           = ".template_shas.json"
)

// =============================================================================

type options struct {
	basePath   string
	githubRepo string
	catalog    *catalog.Catalog
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

// WithCatalog sets a custom catalog api.
func WithCatalog(catalog *catalog.Catalog) Option {
	return func(o *options) {
		o.catalog = catalog
	}
}

// =============================================================================

// Templates manages the template system.
type Templates struct {
	templatePath string
	githubRepo   string
	catalog      *catalog.Catalog
}

// New constructs the template system using defaults paths.
func New(opts ...Option) (*Templates, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	o.basePath = defaults.BaseDir(o.basePath)

	if o.githubRepo == "" {
		o.githubRepo = defaultGithubPath
	}

	catalog, err := catalog.New(catalog.WithBasePath(o.basePath))
	if err != nil {
		return nil, fmt.Errorf("new: unable to create catalog: %w", err)
	}

	templatesPath := filepath.Join(o.basePath, localFolder)

	if err := os.MkdirAll(templatesPath, 0755); err != nil {
		return nil, fmt.Errorf("new: creating templates directory: %w", err)
	}

	t := Templates{
		templatePath: templatesPath,
		githubRepo:   o.githubRepo,
		catalog:      catalog,
	}

	return &t, nil
}

// Catalog returns the catalog being used.
func (t *Templates) Catalog() *catalog.Catalog {
	return t.catalog
}

// TemplatesPath returns the location of the templates path.
func (t *Templates) TemplatesPath() string {
	return t.templatePath
}
