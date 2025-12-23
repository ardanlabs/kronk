// Package template implements the model templater interface for
// template access.
package template

import (
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

// Template provides access to retrieving templates for models.
type Template struct {
}

// New creates a new template.
func New() *Template {
	return &Template{}
}

// Retrieve locates the model id in the catalog system and if the model
// has a template it will be returned.
func (t *Template) Retrieve(modelID string) (model.Template, error) {
	m, err := catalog.RetrieveModelDetails(defaults.BaseDir(""), modelID)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve-model-details: %w", err)
	}

	if m.Template == "" {
		return model.Template{}, errors.New("no template configured")
	}

	content, err := templates.RetrieveFile(defaults.BaseDir(""), m.Template)
	if err != nil {
		return model.Template{}, fmt.Errorf("template-retrieve: %w", err)
	}

	mt := model.Template{
		FileName: m.Template,
		Script:   content,
	}

	return mt, nil
}
