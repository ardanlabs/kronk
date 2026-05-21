// Package bucky is the high-level whisper SDK entry point. It mirrors
// the role sdk/kronk plays for the llama backend: cross-cutting
// initialization, library loading, and the Acquire / Transcribe
// surface.
//
// Init publishes the whisper backend's libraries + catalog factories
// to the cross-backend registry and loads the whisper.cpp shared
// library so subsequent New / NewWithContext calls can construct a
// model handle. Callers that do not use whisper simply skip the Init
// call and the backend is never registered.
package bucky

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/ardanlabs/bucky/pkg/whisper"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
)

var (
	libraryLocation string
	initMu          sync.Mutex
	initDone        bool
)

type initOptions struct {
	libPath string
}

// InitOption represents options for configuring Init.
type InitOption func(*initOptions)

// WithInitLibPath sets a custom library path for whisper.cpp.
func WithInitLibPath(libPath string) InitOption {
	return func(o *initOptions) {
		o.libPath = libPath
	}
}

// Initialized reports whether the bucky backend has been successfully
// initialized. This can be used to determine if the server is running
// in a degraded state due to missing whisper.cpp libraries.
func Initialized() bool {
	initMu.Lock()
	defer initMu.Unlock()

	return initDone
}

// Init initializes the bucky / whisper backend. It registers the
// whisper backend with the cross-backend registry under
// backend.KindWhisper, then resolves the install path for the
// whisper.cpp shared library and loads it. If initialization fails,
// subsequent calls will retry — allowing libraries to be downloaded
// and loaded without restarting the server.
func Init(opts ...InitOption) error {
	initMu.Lock()
	defer initMu.Unlock()

	if initDone {
		return nil
	}

	var o initOptions
	for _, opt := range opts {
		opt(&o)
	}

	// Register the whisper backend with the cross-backend registry
	// up front so CLI / server code that dispatches by kind can
	// construct whisper libs and catalogs even when the shared
	// library load below fails. The kronk backend follows the same
	// pattern.
	if err := backend.Register(backend.Backend{
		Kind: backend.KindWhisper,
		NewLibs: func() (backend.LibsManager, error) {
			return libs.New()
		},
		NewCatalog: func(basePath string) (backend.Catalog, error) {
			return models.NewWithPaths(basePath)
		},
	}); err != nil {
		return fmt.Errorf("init: register whisper backend: %w", err)
	}

	libPath := libs.Path(o.libPath)

	// Windows uses PATH for DLL discovery, Unix uses LD_LIBRARY_PATH.
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("PATH"); !strings.Contains(v, libPath) {
			os.Setenv("PATH", fmt.Sprintf("%s;%s", libPath, v))
		}

	default:
		if v := os.Getenv("LD_LIBRARY_PATH"); !strings.Contains(v, libPath) {
			os.Setenv("LD_LIBRARY_PATH", fmt.Sprintf("%s:%s", libPath, v))
		}
	}

	if err := whisper.Load(libPath); err != nil {
		return fmt.Errorf("init: unable to load whisper library: %w", err)
	}

	libraryLocation = libPath
	initDone = true

	return nil
}
