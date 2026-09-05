// This example prints Malina and stable-diffusion.cpp system information.
// Compatible libraries are downloaded automatically.
//
// Experimental: The Malina SDK public API is subject to change.
//
// The first time you run this program the system will download and install the
// stable-diffusion.cpp libraries.
//
// Run the example like this from the root of the project:
// $ make example-malina-system

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ardanlabs/kronk/sdk/malina"
	"github.com/ardanlabs/kronk/sdk/tools/malina/libs"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := installSystem(); err != nil {
		return fmt.Errorf("unable to install system: %w", err)
	}

	info, err := malina.SystemInfo()
	if err != nil {
		return fmt.Errorf("system info: %w", err)
	}

	fmt.Println("-- stable-diffusion.cpp --")
	fmt.Println("version:              ", info.NativeVersion)
	fmt.Println("physical cores:       ", info.PhysicalCores)
	fmt.Println("GGML backend devices: ", info.BackendDeviceCount)
	fmt.Println()
	fmt.Println("-- System info --")
	fmt.Println(info.Description)

	return nil
}

// =============================================================================

func installSystem() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	libs, err := libs.New(
		libs.WithDetect(ctx, malina.FmtLogger),
		libs.WithValidation(true),
	)
	if err != nil {
		return err
	}

	if _, err := libs.Download(ctx, malina.FmtLogger); err != nil {
		return fmt.Errorf("unable to install stable-diffusion.cpp: %w", err)
	}

	if err := malina.Init(malina.WithLibPath(libs.LibsPath())); err != nil {
		return fmt.Errorf("unable to init Malina: %w", err)
	}

	return nil
}
