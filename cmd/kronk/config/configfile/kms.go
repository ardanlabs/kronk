package configfile

import (
	"errors"
	"fmt"
	"os"
	"strings"

	kronkserver "github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"go.yaml.in/yaml/v3"
)

// SaveKMS writes the changes into the kms: section of the file at path, the
// same way SaveEntry does for a model. The new section is checked against
// the server's config struct first.
func SaveKMS(path string, changes []Change) (string, error) {
	return saveKMS(path, func(data []byte) ([]byte, error) {
		return editKMS(data, changes)
	})
}

// SaveKMSValues replaces the kms: section of the file at path with values, a
// flattened section, as questions.Apply returns it. The new section is
// checked against the server's config struct first.
func SaveKMSValues(path string, values map[string]any) (string, error) {
	return saveKMS(path, func(data []byte) ([]byte, error) {
		return editKMSValues(data, values)
	})
}

func saveKMS(path string, edit func([]byte) ([]byte, error)) (string, error) {
	return saveFile(path, func(data []byte) ([]byte, error) {
		out, err := edit(data)
		if err != nil {
			return nil, err
		}

		kms, err := readKMSData(out)
		if err != nil {
			return nil, err
		}
		if err := kronkserver.CheckKMS(Unflatten(kms)); err != nil {
			return nil, fmt.Errorf("the server would not accept these settings, nothing was written: %w", err)
		}

		return out, nil
	})
}

// PreviewKMS returns the kms: section as SaveKMS would write it.
func PreviewKMS(path string, changes []Change) (string, error) {
	return preview(path, []string{"kms"}, func(data []byte) ([]byte, error) {
		return editKMS(data, changes)
	})
}

// PreviewKMSValues returns the kms: section as SaveKMSValues would write it.
func PreviewKMSValues(path string, values map[string]any) (string, error) {
	return preview(path, []string{"kms"}, func(data []byte) ([]byte, error) {
		return editKMSValues(data, values)
	})
}

// editKMS applies the changes to the kms: section. Only the lines of that
// section are rewritten. An empty section is written as kms: {}, the way the
// default file has it.
func editKMS(data []byte, changes []Change) ([]byte, error) {
	return editKMSWith(data, func(section *yaml.Node) (*yaml.Node, error) {
		if err := applyChanges(section, changes); err != nil {
			return nil, err
		}
		return section, nil
	})
}

// editKMSValues replaces the kms: section with values, a flattened section.
func editKMSValues(data []byte, values map[string]any) ([]byte, error) {
	return editKMSWith(data, func(*yaml.Node) (*yaml.Node, error) {
		return entryFromValues(values)
	})
}

// editKMSWith rewrites the kms: section. edit gets the section as the file
// has it, empty when it doesn't, and returns the section to write.
func editKMSWith(data []byte, edit func(*yaml.Node) (*yaml.Node, error)) ([]byte, error) {
	root, lines, err := parseDoc(data)
	if err != nil {
		return nil, err
	}

	kKey, kVal := lookup(root, "kms")

	var end int
	if kVal != nil {
		end = lastLine(kVal)
	}

	section := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	if kVal != nil && kVal.Kind == yaml.MappingNode {
		section = kVal
	}
	section.Style = 0

	section, err = edit(section)
	if err != nil {
		return nil, err
	}
	if section == nil {
		section = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	section.Style = 0

	rendered := []string{"kms: {}\n"}
	if len(section.Content) > 0 {
		rendered, err = renderEntry("kms", section, 0)
		if err != nil {
			return nil, err
		}
	}

	switch {
	case kKey != nil:
		return []byte(strings.Join(splice(lines, kKey.Line-1, end, rendered), "")), nil

	default:
		// No kms: yet. Put it before models: when there is one, so the server
		// settings stay above the model entries.
		at := len(lines)
		if mKey, _ := lookup(root, "models"); mKey != nil {
			at = mKey.Line - 1
			rendered = append(rendered, "\n")
		} else {
			rendered = append([]string{"\n"}, rendered...)
		}
		return []byte(strings.Join(splice(lines, at, at, rendered), "")), nil
	}
}

// =============================================================================

// ReadKMS returns the flattened kms: section of the file, empty when the file
// or the section doesn't exist.
func ReadKMS(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}

	return readKMSData(data)
}

func readKMSData(data []byte) (map[string]any, error) {
	var doc struct {
		KMS map[string]any `yaml:"kms"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	out := map[string]any{}
	flatten("", doc.KMS, out)

	return out, nil
}
