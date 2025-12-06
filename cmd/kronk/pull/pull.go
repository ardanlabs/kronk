// Package pull provides the pull command code.
package pull

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// Run executes the pull command.
func Run(args []string) error {
	modelPath := defaults.ModelsDir()
	modelURL := args[0]

	var projURL string
	if len(args) == 2 {
		projURL = args[1]
	}

	if _, err := url.ParseRequestURI(modelURL); err != nil {
		return fmt.Errorf("invalid URL: %s", modelURL)
	}

	if projURL != "" {
		if _, err := url.ParseRequestURI(projURL); err != nil {
			return fmt.Errorf("invalid project URL: %s", projURL)
		}
	}

	_, err := tools.DownloadModel(context.Background(), tools.FmtLogger, modelURL, projURL, modelPath)
	if err != nil {
		return fmt.Errorf("unable to install model: %w", err)
	}

	return nil
}
