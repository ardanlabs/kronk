package catalog

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	templateLocalFolder = "templates"
	templateSHAFile     = ".template_shas.json"
)

// templates manages the template system internally for the catalog.
type templates struct {
	templatePath string
	githubRepo   string
}

func newTemplates(basePath string, githubRepo string) (*templates, error) {
	templatesPath := filepath.Join(basePath, templateLocalFolder)

	if err := os.MkdirAll(templatesPath, 0755); err != nil {
		return nil, fmt.Errorf("new-templates: creating templates directory: %w", err)
	}

	t := templates{
		templatePath: templatesPath,
		githubRepo:   githubRepo,
	}

	return &t, nil
}
