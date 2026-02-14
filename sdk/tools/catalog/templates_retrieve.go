package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// RetrieveConfig returns catalog-based configuration for the specified model.
// If the model is not found in the catalog, an error is returned and the
// caller should continue with the user-provided configuration.
func (c *Catalog) RetrieveConfig(modelID string) (model.Config, error) {
	mc := c.ResolvedModelConfig(modelID)

	if err := c.resolveGrammar(&mc.Sampling); err != nil {
		return model.Config{}, fmt.Errorf("retrieve-config: %w", err)
	}

	return mc.ToKronkConfig(), nil
}

// RetrieveTemplate implements the model.Cataloger interface.
func (c *Catalog) RetrieveTemplate(modelID string) (model.Template, error) {
	m, err := c.Details(modelID)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve-template: unable to retrieve model details: %w", err)
	}

	if m.Template == "" {
		return model.Template{}, errors.New("retrieve-template: no template configured")
	}

	content, err := c.retrieveScript(m.Template)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve-template: unable to retrieve template: %w", err)
	}

	mt := model.Template{
		FileName: m.Template,
		Script:   content,
	}

	return mt, nil
}

// retrieveScript returns the contents of the template file.
func (c *Catalog) retrieveScript(templateFileName string) (string, error) {
	filePath := filepath.Join(c.templates.templatePath, templateFileName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("retrieve-script: reading template file: %w", err)
	}

	return string(content), nil
}
