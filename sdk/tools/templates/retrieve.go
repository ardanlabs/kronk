package templates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// RetrieveScript returns the contents of the template file.
func (t *Templates) RetrieveScript(templateFileName string) (string, error) {
	filePath := filepath.Join(t.templatePath, templateFileName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("retrieve-script: reading template file: %w", err)
	}

	return string(content), nil
}

// RetrieveConfig returns catalog-based configuration for the specified model.
// If the model is not found in the catalog, an error is returned and the
// caller should continue with the user-provided configuration.
func (t *Templates) RetrieveConfig(modelID string) (model.Config, error) {
	mc := t.catalog.ResolvedModelConfig(modelID)
	return mc.ToKronkConfig(), nil
}

// RetrieveTemplate implements the model.TemplateRetriever interface.
func (t *Templates) RetrieveTemplate(modelID string) (model.Template, error) {
	m, err := t.catalog.Details(modelID)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve-template: unable to retrieve model details: %w", err)
	}

	if m.Template == "" {
		return model.Template{}, errors.New("retrieve-template: no template configured")
	}

	content, err := t.RetrieveScript(m.Template)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve-template: unable to retrieve template: %w", err)
	}

	mt := model.Template{
		FileName: m.Template,
		Script:   content,
	}

	return mt, nil
}
