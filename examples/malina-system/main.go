// This example prints Malina and stable-diffusion.cpp system information.
//
// Experimental: The Malina SDK public API is subject to change.
//
// Set MALINA_LIB to the stable-diffusion.cpp library directory before running:
//
//	MALINA_LIB=/path/to/libs make example-malina-system
package main

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/malina"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("\nERROR: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := malina.Init(); err != nil {
		return fmt.Errorf("initialize Malina: %w", err)
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
