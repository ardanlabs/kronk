// Package libs provides the libs command code.
package libs

import (
	"context"
	"errors"
	"fmt"

	"github.com/ardanlabs/kronk"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

var ErrInvalidArguments = errors.New("invalid arguments")

// Run executes the pull command.
func Run(args []string) error {
	libPath := defaults.LibsDir()

	processor, err := defaults.Processor()
	if err != nil {
		return err
	}

	_, err = tools.DownloadLibraries(context.Background(), tools.FmtLogger, libPath, processor, true)
	if err != nil {
		return fmt.Errorf("unable to install llama.cpp: %w", err)
	}

	if err := kronk.Init(libPath, kronk.LogSilent); err != nil {
		return fmt.Errorf("installation invalid: %w", err)
	}

	return nil
}
