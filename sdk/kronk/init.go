package kronk

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

// defaultBaseDir and defaultLibFolder mirror the on-disk defaults used by
// the Kronk CLI tooling (sdk/tools/defaults + sdk/tools/libs). They are
// duplicated here so the kronk SDK package does not depend on the tools
// tree. Callers that need richer resolution (per-triple subfolders,
// version.json detection, legacy-layout migration, etc.) should compute
// the path with sdk/tools/libs and pass it via WithLibPath.
const (
	defaultBaseDir   = ".kronk"
	defaultLibFolder = "libraries"
	envKronkLibPath  = "KRONK_LIB_PATH"
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

	libPath := o.libPath
	if libPath == "" {
		libPath = defaultLibPath()
	}

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

	if err := llama.Load(libPath); err != nil {
		return fmt.Errorf("init: unable to load library: %w", err)
	}

	if err := mtmd.Load(libPath); err != nil {
		return fmt.Errorf("init: unable to load mtmd library: %w", err)
	}

	libraryLocation = libPath
	llama.Init()

	if err := model.InitYzmaWorkarounds(libPath); err != nil {
		return fmt.Errorf("unable to init yzma workarounds: %w", err)
	}

	// ---------------------------------------------------------------------

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

	initDone = true

	return nil
}

// defaultLibPath returns the library path to use when the caller did not
// supply one through WithLibPath. KRONK_LIB_PATH takes precedence over the
// built-in default of <home>/.kronk/libraries.
func defaultLibPath() string {
	if v := os.Getenv(envKronkLibPath); v != "" {
		return v
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", defaultBaseDir, defaultLibFolder)
	}

	return filepath.Join(homeDir, defaultBaseDir, defaultLibFolder)
}
