// Package templates provides template support.
package templates

import (
	"fmt"
	"os"
	"path/filepath"

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

// New constructs the template system, using the specified github
// repo path. If the path is empty, the default repo is used.
func New(basePath string, githubRepoPath string) (*Templates, error) {
	if githubRepoPath == "" {
		githubRepoPath = defaultGithubPath
	}

	templatesPath := filepath.Join(basePath, localFolder)

	if err := os.MkdirAll(templatesPath, 0755); err != nil {
		return nil, fmt.Errorf("creating templates directory: %w", err)
	}

	catalog, err := catalog.New(basePath, "")
	if err != nil {
		return nil, fmt.Errorf("catalog new: %w", err)
	}

	t := Templates{
		templatePath:   templatesPath,
		githubRepoPath: githubRepoPath,
		catalog:        catalog,
	}

	return &t, nil
}
