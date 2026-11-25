package models

import "fmt"

// DraftMethod identifies a draft artifact's speculation method. Its zero value is unset.
type DraftMethod struct{ value string }

var draftMethods = make(map[string]DraftMethod)

var (
	// DraftMethodMTP identifies a multi-token prediction companion.
	DraftMethodMTP = newDraftMethod("mtp")
	// DraftMethodDSpark identifies a DSpark companion, not an MTP head.
	DraftMethodDSpark = newDraftMethod("dspark")
	// DraftMethodConventional identifies a conventional speculative drafter.
	DraftMethodConventional = newDraftMethod("conventional")
)

func newDraftMethod(value string) DraftMethod {
	dm := DraftMethod{value: value}
	draftMethods[value] = dm
	return dm
}

// String returns the canonical name.
func (dm DraftMethod) String() string { return dm.value }

// Equal reports whether the methods match.
func (dm DraftMethod) Equal(other DraftMethod) bool { return dm == other }

// IsZero reports whether the method is unset.
func (dm DraftMethod) IsZero() bool { return dm.value == "" }

// MarshalText serializes the method.
func (dm DraftMethod) MarshalText() ([]byte, error) { return []byte(dm.value), nil }

// UnmarshalText parses a registered method.
func (dm *DraftMethod) UnmarshalText(data []byte) error {
	parsed, err := ParseDraftMethod(string(data))
	if err != nil {
		return err
	}
	*dm = parsed
	return nil
}

// ParseDraftMethod parses a registered method or the unset value.
func ParseDraftMethod(value string) (DraftMethod, error) {
	if value == "" {
		return DraftMethod{}, nil
	}
	dm, ok := draftMethods[value]
	if !ok {
		return DraftMethod{}, fmt.Errorf("invalid draft method %q", value)
	}
	return dm, nil
}

// MustParseDraftMethod parses a registered method and panics on an unknown name.
func MustParseDraftMethod(value string) DraftMethod {
	dm, err := ParseDraftMethod(value)
	if err != nil {
		panic(err)
	}
	return dm
}

// CatalogDraft describes a separately installed speculative draft artifact.
// File is the explicit installed GGUF basename; Original is the upstream path.
type CatalogDraft struct {
	Method   DraftMethod `yaml:"method,omitempty" json:"method,omitzero"`
	File     string      `yaml:"file,omitempty" json:"file,omitempty"`
	Original string      `yaml:"original,omitempty" json:"original,omitempty"`
	URL      string      `yaml:"url,omitempty" json:"url,omitempty"`
	Size     int64       `yaml:"size,omitempty" json:"size,omitempty"`
}

func mtpDraft(file, original string) CatalogDraft {
	if file == "" {
		return CatalogDraft{}
	}
	return CatalogDraft{Method: DraftMethodMTP, File: file, Original: original}
}

func (d CatalogDraft) isMTP() bool { return d.Method == DraftMethodMTP && d.File != "" }
