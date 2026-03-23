package loaders

import (
	"embed"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// EmbedFSLoader implements access to files in embed.FS instance
type EmbedFSLoader struct {
	root string
	fs   *embed.FS
}

// NewEmbedFSLoader created new EmbedFSLoader and allows templates to be loaded from embed.FS instance
func NewEmbedFSLoader(root string, fs *embed.FS) (Loader, error) {
	if _, err := fs.Open(root); err != nil {
		return nil, err
	}
	loader := &EmbedFSLoader{
		root: root,
		fs:   fs,
	}
	return loader, nil
}

func (e *EmbedFSLoader) Read(path string) (io.Reader, error) {
	resolved, err := e.Resolve(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve name '%s': %w", path, err)
	}

	return e.fs.Open(strings.TrimLeft(resolved, "/"))
}

func (e *EmbedFSLoader) Resolve(path string) (string, error) {
	if strings.HasPrefix(path, "/") {
		return path, nil
	}
	resolved := filepath.Clean(strings.Join([]string{e.root, path}, "/"))
	if _, err := e.fs.Open(strings.TrimLeft(resolved, "/")); err != nil {
		return "", fmt.Errorf("unknown resolved path '%s': %w", resolved, err)
	}
	return resolved, nil
}

func (e *EmbedFSLoader) Inherit(from string) (Loader, error) {
	if from == "" {
		return NewEmbedFSLoader(e.root, e.fs)
	}
	resolvedFrom, err := e.Resolve(from)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve '%s': %s", from, err)
	}
	components := strings.Split(resolvedFrom, "/")
	var root string
	if len(components) < 2 {
		root = "/"
	} else {
		root = strings.Join(components[:len(components)-1], "/")
	}
	return NewEmbedFSLoader(root, e.fs)
}
