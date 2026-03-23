package loaders

import (
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
)

// MemoryLoader represents a basic in-memory loader
type memoryLoader struct {
	root    string
	content map[string]string
}

func MustNewMemoryLoader(content map[string]string) Loader {
	loader, err := NewMemoryLoader(content)
	if err != nil {
		log.Panic(err)
	}
	return loader
}

// NewMemoryLoader creates a new MemoryLoader and allows
// templates to be loaded from memory.
func NewMemoryLoader(content map[string]string) (Loader, error) {
	root := ""
	for key := range content {
		if !strings.HasPrefix(key, "/") {
			return nil, fmt.Errorf("all keys must start with '/' but the following does not: '%s'", key)
		}
		if root == "" {
			root = key
			continue
		}
		for !strings.HasPrefix(key, root) && root != "" {
			root = root[:len(root)-1]
		}
	}
	return &memoryLoader{
		root:    root,
		content: content,
	}, nil
}

func (m *memoryLoader) Inherit(from string) (Loader, error) {
	root := m.root
	if from != "" {
		resolvedFrom, err := m.Resolve(from)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve '%s': %s", from, err)
		}
		components := strings.Split(resolvedFrom, "/")
		if len(components) < 2 {
			root = "/"
		} else {
			root = strings.Join(components[:len(components)-1], "/")
		}
	}
	return &memoryLoader{
		content: m.content,
		root:    root,
	}, nil
}

func (m *memoryLoader) Read(path string) (io.Reader, error) {
	resolved, err := m.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve name '%s': %s", path, err)
	}

	data, ok := m.content[resolved]
	if !ok {
		return nil, fmt.Errorf("unknown path: '%s'", resolved)
	}
	return strings.NewReader(data), nil
}

func (m *memoryLoader) Resolve(path string) (string, error) {
	if strings.HasPrefix(path, "/") {
		return path, nil
	}
	resolved := filepath.Clean(strings.Join([]string{m.root, path}, "/"))
	if _, ok := m.content[resolved]; !ok {
		return "", fmt.Errorf("unknown resolved path: '%s'", resolved)
	}

	return resolved, nil
}
