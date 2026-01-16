package catalog

import (
	"time"
)

// Metadata represents extra information about the model.
type Metadata struct {
	Created     time.Time `yaml:"created"`
	Collections string    `yaml:"collections"`
	Description string    `yaml:"description"`
}

// Capabilities represents the capabilities of a model.
type Capabilities struct {
	Endpoint  string `yaml:"endpoint"`
	Images    bool   `yaml:"images"`
	Audio     bool   `yaml:"audio"`
	Video     bool   `yaml:"video"`
	Streaming bool   `yaml:"streaming"`
	Reasoning bool   `yaml:"reasoning"`
	Tooling   bool   `yaml:"tooling"`
	Embedding bool   `yaml:"embedding"`
	Rerank    bool   `yaml:"rerank"`
}

// File represents the actual file url and size.
type File struct {
	URL  string `yaml:"url"`
	Size string `yaml:"size"`
}

// Files represents file information for a model.
type Files struct {
	Models []File `yaml:"models"`
	Proj   File   `yaml:"proj"`
}

// ToModelURLS converts a slice of File to a string of the URLs.
func (f Files) ToModelURLS() []string {
	models := make([]string, len(f.Models))

	for i, file := range f.Models {
		models[i] = file.URL
	}

	return models
}

// Model represents information for a model.
type Model struct {
	ID           string       `yaml:"id"`
	Category     string       `yaml:"category"`
	OwnedBy      string       `yaml:"owned_by"`
	ModelFamily  string       `yaml:"model_family"`
	WebPage      string       `yaml:"web_page"`
	GatedModel   bool         `yaml:"gated_model"`
	Template     string       `yaml:"template"`
	Files        Files        `yaml:"files"`
	Capabilities Capabilities `yaml:"capabilities"`
	Metadata     Metadata     `yaml:"metadata"`
	Downloaded   bool
	Validated    bool
}

// CatalogModels represents a set of models for a given catalog.
type CatalogModels struct {
	Name   string  `yaml:"catalog"`
	Models []Model `yaml:"models"`
}
