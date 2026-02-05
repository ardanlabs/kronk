package kronk

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/nikolalohinski/gonja/v2"
)

var (
	libraryLocation string
	initOnce        sync.Once
	initErr         error
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

// Init initializes the Kronk backend support.
func Init(opts ...InitOption) error {
	initOnce.Do(func() {
		var o initOptions
		for _, opt := range opts {
			opt(&o)
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

		if err := llama.Load(libPath); err != nil {
			initErr = fmt.Errorf("init: unable to load library: %w", err)
			return
		}

		if err := mtmd.Load(libPath); err != nil {
			initErr = fmt.Errorf("init: unable to load mtmd library: %w", err)
			return
		}

		libraryLocation = libPath
		llama.Init()

		if err := model.InitYzmaWorkarounds(libPath); err != nil {
			initErr = fmt.Errorf("unable to init yzma workarounds: %w", err)
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

		gonja.SetLoggerOutput(io.Discard)
	})

	return initErr
}
