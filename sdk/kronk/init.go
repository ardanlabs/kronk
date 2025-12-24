package kronk

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
	"github.com/nikolalohinski/gonja/v2"
)

// Init initializes the Kronk backend suport.
func Init() error {
	return InitWithSettings("", LogSilent)
}

// InitWithSettings initializes the Kronk backend suport.
func InitWithSettings(libPath string, logLevel LogLevel) error {
	initOnce.Do(func() {
		libPath := libs.Path(libPath)

		if v := os.Getenv("LD_LIBRARY_PATH"); !strings.Contains(v, libPath) {
			os.Setenv("LD_LIBRARY_PATH", fmt.Sprintf("%s:%s", libPath, v))
		}

		if err := llama.Load(libPath); err != nil {
			initErr = fmt.Errorf("unable to load library: %w", err)
			return
		}

		if err := mtmd.Load(libPath); err != nil {
			initErr = fmt.Errorf("unable to load mtmd library: %w", err)
			return
		}

		libraryLocation = libPath
		llama.Init()

		// ---------------------------------------------------------------------

		if logLevel < 1 || logLevel > 2 {
			logLevel = LogSilent
		}

		switch logLevel {
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
