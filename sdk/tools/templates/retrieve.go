package templates

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// RetrieveTemplate returns the contents of the template file.
func (t *Templates) RetrieveTemplate(templateFileName string) (string, error) {
	filePath := filepath.Join(t.templatePath, templateFileName)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("retrieve-template: reading template file: %w", err)
	}

	return string(content), nil
}

// Retrieve implements the model.TemplateRetriever interface.
func (t *Templates) Retrieve(modelID string) (model.Template, error) {
	m, err := t.catalog.Details(modelID)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve: unable to retrieve model details: %w", err)
	}

	if m.Template == "" {
		return model.Template{}, errors.New("retrieve: no template configured")
	}

	content, err := t.RetrieveTemplate(m.Template)
	if err != nil {
		return model.Template{}, fmt.Errorf("retrieve: unable to retrieve template: %w", err)
	}

	mt := model.Template{
		FileName: m.Template,
		Script:   content,
	}

	return mt, nil
}
