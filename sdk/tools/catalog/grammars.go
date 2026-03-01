package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/tools/github"
)

const (
	grammarLocalFolder = "grammars"
	grammarSHAFile     = ".grammar_shas.json"
)

// grammars manages the grammar system internally for the catalog.
type grammars struct {
	grammarPath string
	githubRepo  string
	ghClient    *github.Client
}

func newGrammars(basePath string, githubRepo string, ghClient *github.Client) (*grammars, error) {
	grammarsPath := filepath.Join(basePath, grammarLocalFolder)

	if err := os.MkdirAll(grammarsPath, 0755); err != nil {
		return nil, fmt.Errorf("new-grammars: creating grammars directory: %w", err)
	}

	g := grammars{
		grammarPath: grammarsPath,
		githubRepo:  githubRepo,
		ghClient:    ghClient,
	}

	return &g, nil
}
