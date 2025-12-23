// Package update provides the catalog update command code.
package update

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

func runWeb() error {
	fmt.Println("catalog update: not implemented")
	return nil
}

func runLocal(catalog *catalog.Catalog) error {
	fmt.Println("Starting Update")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := catalog.Download(ctx); err != nil {
		return fmt.Errorf("download: %w", err)
	}

	fmt.Println("Catalog Updated")

	return nil
}
