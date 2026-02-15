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

// TemplateFileInfo provides information about a template file.
type TemplateFileInfo struct {
	Name string
	Size int64
}

// TemplatePath returns the location of the templates directory.
func (c *Catalog) TemplatePath() string {
	return c.templates.templatePath
}

// ListTemplates returns information about all .jinja template files.
func (c *Catalog) ListTemplates() ([]TemplateFileInfo, error) {
	entries, err := os.ReadDir(c.templates.templatePath)
	if err != nil {
		return nil, fmt.Errorf("list-templates: reading directory: %w", err)
	}

	var list []TemplateFileInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jinja" {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		list = append(list, TemplateFileInfo{
			Name: entry.Name(),
			Size: info.Size(),
		})
	}

	return list, nil
}

// ReadTemplate returns the content of the specified template file.
func (c *Catalog) ReadTemplate(name string) (string, error) {
	if filepath.Base(name) != name || strings.Contains(name, "..") {
		return "", fmt.Errorf("read-template: invalid template name: %s", name)
	}

	if filepath.Ext(name) != ".jinja" {
		return "", fmt.Errorf("read-template: template must have .jinja extension: %s", name)
	}

	content, err := os.ReadFile(filepath.Join(c.templates.templatePath, name))
	if err != nil {
		return "", fmt.Errorf("read-template: reading file: %w", err)
	}

	return string(content), nil
}

// SaveTemplate writes a template file to the templates directory.
func (c *Catalog) SaveTemplate(name string, script string) error {
	if filepath.Base(name) != name || strings.Contains(name, "..") {
		return fmt.Errorf("save-template: invalid template name: %s", name)
	}

	if filepath.Ext(name) != ".jinja" {
		return fmt.Errorf("save-template: template must have .jinja extension: %s", name)
	}

	const maxTemplateSize = 256 * 1024
	if len(script) > maxTemplateSize {
		return fmt.Errorf("save-template: template exceeds maximum size of 256KB")
	}

	filePath := filepath.Join(c.templates.templatePath, name)

	if err := os.WriteFile(filePath, []byte(script), 0644); err != nil {
		return fmt.Errorf("save-template: writing file: %w", err)
	}

	return nil
}
