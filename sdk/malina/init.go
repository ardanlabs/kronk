// Package malina provides a concurrency-safe API for generating images with
// stable-diffusion.cpp through the Malina raw bindings.
//
// Experimental: This package's public API is subject to change.
package malina

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk"
	backendtools "github.com/ardanlabs/kronk/sdk/tools/backend"
	malinalibs "github.com/ardanlabs/kronk/sdk/tools/malina/libs"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/ardanlabs/malina/pkg/sd"
)

// Version contains the current version of the Malina SDK package.
const Version = kronk.Version

var initState struct {
	sync.Mutex
	done bool
	path string
}

type initOptions struct {
	libPath  string
	logLevel LogLevel
	progress ProgressFunc
}

// InitOption represents options for configuring Init.
type InitOption func(*initOptions)

// WithLibPath sets a custom library path for stable-diffusion.cpp.
func WithLibPath(libPath string) InitOption {
	return func(o *initOptions) {
		o.libPath = libPath
	}
}

// WithLogLevel sets the log level for stable-diffusion.cpp and GGML. Logging
// is silent by default. Pass LogNormal to write native diagnostics to stderr.
func WithLogLevel(logLevel LogLevel) InitOption {
	return func(o *initOptions) {
		o.logLevel = logLevel
	}
}

// WithProgress replaces stable-diffusion.cpp's native terminal progress with
// progress. Pass DiscardProgress to suppress model loading and generation
// progress. A nil function retains the native terminal display.
func WithProgress(progress ProgressFunc) InitOption {
	return func(o *initOptions) {
		o.progress = progress
	}
}

// Initialized reports whether the Malina backend has been successfully
// initialized.
func Initialized() bool {
	initState.Lock()
	defer initState.Unlock()

	return initState.done
}

// Init registers Malina tooling, then loads stable-diffusion.cpp and its
// dynamic backends. KRONK_MALINA_LIB_PATH (or legacy MALINA_LIB) is used when
// no library path is supplied.
func Init(opts ...InitOption) error {
	initState.Lock()
	defer initState.Unlock()

	var o initOptions
	for _, opt := range opts {
		opt(&o)
	}
	if err := backendtools.Register(backendtools.Backend{
		Kind:       backendtools.KindStableDiffusion,
		NewLibs:    func() (backendtools.LibsManager, error) { return malinalibs.New() },
		NewCatalog: func(basePath string) (backendtools.Catalog, error) { return malinamodels.NewWithPaths(basePath) },
	}); err != nil {
		return fmt.Errorf("init: register backend: %w", err)
	}
	if initState.done && o.libPath == "" {
		return nil
	}
	o.libPath = malinalibs.Path(o.libPath)

	if initState.done {
		if o.libPath != "" && o.libPath != initState.path {
			return fmt.Errorf("init: stable-diffusion already initialized from %q, cannot use %q", initState.path, o.libPath)
		}
		return nil
	}
	if o.logLevel < LogSilent || o.logLevel > LogNormal {
		o.logLevel = LogSilent
	}
	switch o.logLevel {
	case LogSilent:
		sd.SetLogCallback(func(sd.LogLevel, string) {})
	default:
		sd.SetLogCallback(func(_ sd.LogLevel, text string) {
			fmt.Fprintln(os.Stderr, text)
		})
	}
	if o.progress == nil {
		sd.SetProgressCallback(nil)
	} else {
		sd.SetProgressCallback(func(step int, steps int, secondsPerStep float32) {
			o.progress(step, steps, secondsPerStep)
		})
	}
	if err := sd.Load(o.libPath); err != nil {
		return fmt.Errorf("init: unable to load stable-diffusion library: %w", err)
	}
	if sd.GGMLBackendDeviceCount() <= 0 {
		if err := sd.Init(o.libPath); err != nil {
			return fmt.Errorf("init: unable to initialize stable-diffusion backends: %w", err)
		}
	}

	initState.done = true
	initState.path = o.libPath

	return nil
}

// SystemDiagnostics contains native library and host diagnostics.
type SystemDiagnostics struct {
	NativeVersion      string
	PhysicalCores      int32
	BackendDeviceCount int
	Description        string
}

// SystemInfo returns native library and host diagnostics after initialization.
func SystemInfo() (SystemDiagnostics, error) {
	if !Initialized() {
		return SystemDiagnostics{}, errors.New("system-info: malina is not initialized")
	}
	return systemDiagnostics(), nil
}

func systemDiagnostics() SystemDiagnostics {
	return SystemDiagnostics{
		NativeVersion:      sd.Version(),
		PhysicalCores:      sd.NumPhysicalCores(),
		BackendDeviceCount: sd.GGMLBackendDeviceCount(),
		Description:        sd.SystemInfo(),
	}
}
