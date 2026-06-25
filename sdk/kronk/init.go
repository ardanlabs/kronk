package kronk

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/backend"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

var (
	libraryLocation string
	initMu          sync.Mutex
	initDone        bool
)

type initOptions struct {
	libPath  string
	logLevel LogLevel
}

// InitOption represents options for configuring Init.
type InitOption func(*initOptions)

// WithLibPath sets a custom library path.
func WithLibPath(libPath string) InitOption {
	return func(o *initOptions) {
		o.libPath = libPath
	}
}

// WithLogLevel sets the log level for the backend.
func WithLogLevel(logLevel LogLevel) InitOption {
	return func(o *initOptions) {
		o.logLevel = logLevel
	}
}

// Initialized reports whether the Kronk backend has been successfully
// initialized. This can be used to determine if the server is running
// in a degraded state due to missing libraries.
func Initialized() bool {
	initMu.Lock()
	defer initMu.Unlock()

	return initDone
}

// Init initializes the Kronk backend support. If initialization fails,
// subsequent calls will retry, allowing libraries to be downloaded and
// loaded without restarting the server.
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

	// Register the llama backend with the cross-backend registry so
	// CLI / server code that dispatches by kind can construct llama
	// libs and catalogs without importing the concrete packages
	// directly. Registration is idempotent; subsequent Init calls (or
	// other backends registering themselves) compose cleanly.
	if err := backend.Register(backend.Backend{
		Kind: backend.KindLlama,
		NewLibs: func() (backend.LibsManager, error) {
			return libs.New()
		},
		NewCatalog: func(basePath string) (backend.Catalog, error) {
			return models.NewWithPaths(basePath)
		},
	}); err != nil {
		return fmt.Errorf("init: register llama backend: %w", err)
	}

	// NOTE: This is the only place where the sdk/kronk is reaching for a
	// tools package for support. There is no way around this and I can't move
	// the libs package out of tools for backwards compatibility. This is a
	// function of wanting a separation between inference and supporting
	// inference. The libs package is still a support package, but sdk/kronk
	// needs the support in this case as well.
	libPath := libs.Path(o.libPath)

	// Windows uses PATH for DLL discovery, Unix uses LD_LIBRARY_PATH.
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("PATH"); !strings.Contains(v, libPath) {
			os.Setenv("PATH", fmt.Sprintf("%s;%s", libPath, v))
		}

		// PATH alone is not enough on Windows: the system directory (System32)
		// is searched before PATH, so a stale same-named copy of a bundled
		// dependency (e.g. libomp140.x86_64.dll) shadows ours and fails the
		// load with "the specified procedure could not be found". Preload our
		// libraries from libPath so they (and their dependency tree) win.
		// Best-effort: on failure the PATH entry above remains the fallback, so
		// the error is intentionally non-fatal.
		_ = preloadLibraries(libPath)

	default:
		if v := os.Getenv("LD_LIBRARY_PATH"); !strings.Contains(v, libPath) {
			os.Setenv("LD_LIBRARY_PATH", fmt.Sprintf("%s:%s", libPath, v))
		}
	}

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("init: unable to load library: %w", err)
	}

	if err := mtmd.Load(libPath); err != nil {
		return fmt.Errorf("init: unable to load mtmd library: %w", err)
	}

	// Install log callbacks BEFORE llama.Init(). Newer ggml/llama.cpp
	// builds eagerly initialize backend devices (e.g. Metal) inside
	// llama.Init(), and any log messages emitted during that init would
	// otherwise bypass the silencer and print to stderr.
	if o.logLevel < 1 || o.logLevel > 2 {
		o.logLevel = LogSilent
	}

	switch o.logLevel {
	case LogSilent:
		llama.LogSet(llama.LogSilent())
		mtmd.LogSet(llama.LogSilent())
	default:
		llama.LogSet(llama.LogNormal)
		mtmd.LogSet(llama.LogNormal)
	}

	libraryLocation = libPath

	// Inline of llama.Init so we can gate GGMLBackendLoadAllFromPath on
	// the registry being empty. If bucky/whisper was initialized first
	// from a sibling lib directory, its libggml-*.{so,dylib,dll} files
	// have already self-registered via different absolute paths; calling
	// load-all from the kronk lib dir here would dlopen a second copy of
	// each backend and publish duplicate Vulkan0 / Vulkan1 / CPU entries
	// that the resman snapshot rejects with "duplicate device name".
	llama.BackendInit()
	if llama.GGMLBackendDeviceCount() == 0 {
		if err := llama.GGMLBackendLoadAllFromPath(libPath); err != nil {
			return fmt.Errorf("init: unable to load ggml backends: %w", err)
		}
	}

	if err := model.InitYzmaWorkarounds(libPath); err != nil {
		return fmt.Errorf("unable to init yzma workarounds: %w", err)
	}

	initDone = true

	return nil
}
