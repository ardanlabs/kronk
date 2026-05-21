package libs

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/bucky"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/libs"
)

// runDefault installs the whisper.cpp libraries for the current host
// using the well-known default version (or the supplied version) and
// then initializes the bucky runtime against the install path so the
// libraries can be loaded.
func runDefault(version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	lib, err := libs.New(
		libs.WithLibPath(""),
		libs.WithVersion(version),
	)
	if err != nil {
		return fmt.Errorf("bucky libs: new: %w", err)
	}

	if _, err := lib.Download(ctx, bucky.FmtLogger); err != nil {
		return fmt.Errorf("bucky libs: unable to install whisper.cpp: %w", err)
	}

	if err := bucky.Init(bucky.WithInitLibPath(lib.LibsPath())); err != nil {
		return fmt.Errorf("bucky libs: installation invalid: %w", err)
	}

	return nil
}
