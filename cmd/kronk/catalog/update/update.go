// Package update provides the catalog update command code.
package update

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

func runWeb() error {
	fmt.Println("catalog update: not implemented")
	return nil
}

func runLocal() error {
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
