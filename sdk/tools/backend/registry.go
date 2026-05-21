package backend

import (
	"fmt"
	"slices"
	"sync"
)

// Backend bundles the factories cross-backend code needs to construct a
// concrete backend's primitives without compile-time knowledge of which
// backend it is talking to.
//
// Factories return the shared interfaces (LibsManager, Catalog) rather
// than the concrete types so callers stay decoupled from each backend's
// implementation package.
type Backend struct {
	// Kind identifies the backend. It must match one of the Kind*
	// constants and is the key used by Register/Get.
	Kind string

	// NewLibs constructs the backend's LibsManager using its defaults.
	NewLibs func() (LibsManager, error)

	// NewCatalog constructs the backend's Catalog rooted at basePath.
	// An empty basePath means "use the backend's default base path".
	NewCatalog func(basePath string) (Catalog, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]Backend{}
)

// Register adds a backend to the process-wide registry. Registering the
// same kind twice replaces the previous entry; this keeps the call
// idempotent so binaries that invoke Init-style helpers more than once
// (tests, repeated bootstraps) do not fail.
//
// Returns an error when Kind is empty or any required factory is nil.
func Register(b Backend) error {
	if b.Kind == "" {
		return fmt.Errorf("backend: register: kind is required")
	}
	if b.NewLibs == nil {
		return fmt.Errorf("backend: register: %q: NewLibs is required", b.Kind)
	}
	if b.NewCatalog == nil {
		return fmt.Errorf("backend: register: %q: NewCatalog is required", b.Kind)
	}

	registryMu.Lock()
	defer registryMu.Unlock()

	registry[b.Kind] = b

	return nil
}

// Get returns the backend registered under kind. The bool is false when
// no backend with that kind is registered.
func Get(kind string) (Backend, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	b, ok := registry[kind]

	return b, ok
}

// Kinds returns every registered backend kind, sorted ascending. The
// returned slice is a fresh copy and may be modified by the caller.
func Kinds() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()

	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}

	slices.Sort(out)

	return out
}
