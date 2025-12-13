// Package update provides the catalog update command code.
package update

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools/catalog"
)

// RunWeb executes the catalog update command against the model server.
func RunWeb(args []string) error {
	fmt.Println("catalog update: not implemented")
	return nil
}

// RunLocal executes the catalog update command locally.
func RunLocal(args []string) error {
	basePath := defaults.BaseDir("")

	fmt.Println("Starting Update")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := catalog.Download(ctx, basePath); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	fmt.Println("Catalog Updated")

	return nil
}
