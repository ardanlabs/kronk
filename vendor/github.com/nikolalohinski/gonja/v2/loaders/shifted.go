package loaders

import (
	"bytes"
	"fmt"
	"io"
	"log"
)

// shiftedLoader represents a wrapping loader on top of an existing one
// where the root template is passed as code, and any loader calls
// to other paths than the root will use the provided sub-loader whereas
// accessing the root template will always return what was passed as code
type shiftedLoader struct {
	loader      Loader
	rootID      string
	rootContent []byte
}

// MustNewShiftedLoader creates a new shifted loader instance
// and panics if there's any error during instantiation
func MustNewShiftedLoader(rootID string, rootContent io.Reader, subLoader Loader) Loader {
	loader, err := NewShiftedLoader(rootID, rootContent, subLoader)
	if err != nil {
		log.Panic(err)
	}
	return loader
}

// NewShiftedLoader creates a new shiftedLoader and allows wrapping an existing
// loader where the root template is passed as code, and any loader calls
// to other paths than the root will use the provided sub-loader whereas
// accessing the root template will always return what was passed as code
func NewShiftedLoader(rootID string, rootContent io.Reader, loader Loader) (Loader, error) {
	content, err := io.ReadAll(rootContent)
	if err != nil {
		return nil, fmt.Errorf("failed to read root content: %s", err)
	}

	return &shiftedLoader{
		rootID:      rootID,
		rootContent: content,
		loader:      loader,
	}, nil
}

// Create a new loader from the current one, relatively to the given identifier
func (f *shiftedLoader) Inherit(from string) (Loader, error) {
	loader, err := f.loader.Inherit(from)
	if err != nil {
		return nil, fmt.Errorf("failed to inherit file system loader: %s", err)
	}
	return &shiftedLoader{
		rootID:      f.rootID,
		rootContent: f.rootContent,
		loader:      loader,
	}, nil
}

// Read returns an io.Reader where the template's content can be read from
func (f *shiftedLoader) Read(identifier string) (io.Reader, error) {
	resolvedID, err := f.Resolve(identifier)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve '%s': %s", identifier, err)
	}
	if resolvedID == f.rootID {
		buffer := bytes.NewBuffer(f.rootContent)
		return buffer, nil
	}
	return f.loader.Read(identifier)
}

// Resolve the given identifier in the current context
func (f *shiftedLoader) Resolve(identifier string) (string, error) {
	if identifier == f.rootID {
		return identifier, nil
	}
	return f.loader.Resolve(identifier)
}
