package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// RetrieveConfig returns catalog-based configuration for the specified model.
// If the model is not found in the catalog, an error is returned and the
// caller should continue with the user-provided configuration.
func (c *Catalog) RetrieveConfig(modelID string) (model.Config, error) {
	mc := c.ResolvedModelConfig(modelID)

	if err := c.ResolveGrammar(&mc.Sampling); err != nil {
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

// TemplateFiles returns a sorted list of available template filenames.
func (c *Catalog) TemplateFiles() ([]string, error) {
	entries, err := os.ReadDir(c.templates.templatePath)
	if err != nil {
		return nil, fmt.Errorf("template-files: reading templates directory: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		files = append(files, name)
	}

	return files, nil
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
