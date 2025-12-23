// Package templates provides template support.
package templates

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

const (
	defaultGithubPath = "https://api.github.com/repos/ardanlabs/kronk_catalogs/contents/templates"
	localFolder       = "templates"
	shaFile           = ".template_shas.json"
)

// Templates manages the template system.
type Templates struct {
	templatePath   string
	githubRepoPath string
	catalog        *catalog.Catalog
}

// New constructs the template system using defaults paths.
func New() (*Templates, error) {
	basePath := defaults.BaseDir("")

	catalog, err := catalog.NewWithSettings(basePath, "")
	if err != nil {
		return nil, fmt.Errorf("catalog new: %w", err)
	}

	return NewWithSettings("", "", catalog)
}

// NewWithCatalog constructs the template system using defaults paths.
func NewWithCatalog(catalog *catalog.Catalog) (*Templates, error) {
	return NewWithSettings("", "", catalog)
}

// NewWithSettings constructs the template system, using the specified settings.
// If either path is empty, the default paths are used.
func NewWithSettings(basePath string, githubRepoPath string, catalog *catalog.Catalog) (*Templates, error) {
	basePath = defaults.BaseDir(basePath)

	if githubRepoPath == "" {
		githubRepoPath = defaultGithubPath
	}

	templatesPath := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(templatesPath, 0755); err != nil {
		return nil, fmt.Errorf("creating templates directory: %w", err)
	}

	t := Templates{
		templatePath:   templatesPath,
		githubRepoPath: githubRepoPath,
		catalog:        catalog,
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
