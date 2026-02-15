package playgroundapp

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

// playgroundCataloger implements model.Cataloger for playground sessions.
// It provides template overrides without modifying the catalog's prompts.go code.
type playgroundCataloger struct {
	modelID        string
	templateMode   string
	templateName   string
	templateScript string
	catalog        model.Cataloger
}

// RetrieveTemplate returns the template based on the configured mode.
// In custom mode, it returns the user-provided script directly.
// In builtin mode with a specific template name, it loads that file.
// Otherwise, it delegates to the real catalog for automatic resolution.
func (pc *playgroundCataloger) RetrieveTemplate(modelID string) (model.Template, error) {
	switch pc.templateMode {
	case "custom":
		return model.Template{
			FileName: "<custom>",
			Script:   pc.templateScript,
		}, nil

	default:
		if pc.templateName != "" {
			script, err := pc.catalog.(*catalog.Catalog).ReadTemplate(pc.templateName)
			if err != nil {
				return model.Template{}, fmt.Errorf("loading template %s: %w", pc.templateName, err)
			}
			return model.Template{
				FileName: pc.templateName,
				Script:   script,
			}, nil
		}

		return pc.catalog.RetrieveTemplate(modelID)
	}
}

// RetrieveConfig returns an error so that model.NewModel skips
// applyCatalogConfig() and uses only the pre-built config passed to kronk.New.
func (pc *playgroundCataloger) RetrieveConfig(modelID string) (model.Config, error) {
	return model.Config{}, errors.New("playground: config bypass")
}
