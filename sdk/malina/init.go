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
	libPath string
}

// InitOption represents options for configuring Init.
type InitOption func(*initOptions)

// WithLibPath sets a custom library path for stable-diffusion.cpp.
func WithLibPath(libPath string) InitOption {
	return func(o *initOptions) {
		o.libPath = libPath
	}
}

// Initialized reports whether the Malina backend has been successfully
// initialized.
func Initialized() bool {
	initState.Lock()
	defer initState.Unlock()

	return initState.done
}

// Init loads stable-diffusion.cpp and registers its dynamic backends. The
// MALINA_LIB environment variable is used when no library path is supplied.
func Init(opts ...InitOption) error {
	initState.Lock()
	defer initState.Unlock()

	var o initOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.libPath == "" {
		o.libPath = os.Getenv("MALINA_LIB")
	}

	if initState.done {
		if o.libPath != "" && o.libPath != initState.path {
			return fmt.Errorf("init: stable-diffusion already initialized from %q, cannot use %q", initState.path, o.libPath)
		}
		return nil
	}
	if o.libPath == "" {
		return errors.New("init: library path is required (set MALINA_LIB or use WithLibPath)")
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

	info := SystemDiagnostics{
		NativeVersion:      sd.Version(),
		PhysicalCores:      sd.NumPhysicalCores(),
		BackendDeviceCount: sd.GGMLBackendDeviceCount(),
		Description:        sd.SystemInfo(),
	}

	return info, nil
}
