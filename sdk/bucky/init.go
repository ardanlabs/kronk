// Package bucky is the high-level whisper SDK entry point. It mirrors
// the role sdk/kronk plays for the llama backend: cross-cutting
// initialization, library loading, and (in later steps) the
// Acquire / Transcribe surface.
//
// At step 4 of the bucky integration this package exposes only Init,
// which publishes the whisper backend's libraries + catalog factories
// to the cross-backend registry. Callers that do not use whisper
// simply skip the Init call and the backend is never registered.
package bucky

import (
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

// Init registers the whisper backend with the cross-backend registry
// under backend.KindWhisper so CLI / server code that dispatches by
// kind can construct whisper libs and catalogs without importing the
// concrete packages directly. Registration is idempotent — subsequent
// calls replace the previous entry — so binaries that call Init more
// than once (tests, repeated bootstraps) do not fail.
func Init() error {
	return backend.Register(backend.Backend{
		Kind: backend.KindWhisper,
		NewLibs: func() (backend.LibsManager, error) {
			return libs.New()
		},
		NewCatalog: func(basePath string) (backend.Catalog, error) {
			return models.NewWithPaths(basePath)
		},
	})
}
