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
}

// File represents the actual file url and size.
type File struct {
	URL  string `yaml:"url"`
	Size string `yaml:"size"`
}

// Files represents file information for a model.
type Files struct {
	Model File `yaml:"model"`
	Proj  File `yaml:"proj"`
	Jinja File `yaml:"jinja"`
}

// Model represents information for a model.
type Model struct {
	ID           string       `yaml:"id"`
	Category     string       `yaml:"category"`
	OwnedBy      string       `yaml:"owned_by"`
	ModelFamily  string       `yaml:"model_family"`
	WebPage      string       `yaml:"web_page"`
	Files        Files        `yaml:"files"`
	Capabilities Capabilities `yaml:"capabilities"`
	Metadata     Metadata     `yaml:"metadata"`
	Downloaded   bool
}

// Catalog represents a set of models for a given catalog.
type Catalog struct {
	Name   string  `yaml:"catalog"`
	Models []Model `yaml:"models"`
}
