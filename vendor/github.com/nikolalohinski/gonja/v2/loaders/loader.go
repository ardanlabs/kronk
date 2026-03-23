// Package loaders provides template loading implementations.
package loaders

import (
	"io"
)

// Loader is a wrapper interface to interact with a storage system for templates
type Loader interface {
	// Read returns an io.Reader where the template's content can be read from.
	Read(path string) (io.Reader, error)

	// Resolve the given path in the current context
	Resolve(path string) (string, error)

	// Create a new loader from the current one, relatively to the given path
	Inherit(from string) (Loader, error)
}
