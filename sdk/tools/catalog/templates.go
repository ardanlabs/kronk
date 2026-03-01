package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/github"
)

const (
	templateLocalFolder = "templates"
	templateSHAFile     = ".template_shas.json"
)

// templates manages the template system internally for the catalog.
type templates struct {
	templatePath string
	githubRepo   string
	ghClient     *github.Client
}

func newTemplates(basePath string, githubRepo string, ghClient *github.Client) (*templates, error) {
	templatesPath := filepath.Join(basePath, templateLocalFolder)

	if err := os.MkdirAll(templatesPath, 0755); err != nil {
		return nil, fmt.Errorf("new-templates: creating templates directory: %w", err)
	}

	t := templates{
		templatePath: templatesPath,
		githubRepo:   githubRepo,
		ghClient:     ghClient,
	}

	return &t, nil
}
