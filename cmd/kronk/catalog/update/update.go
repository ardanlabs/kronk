// Package update provides the catalog update command code.
package update

import (
	"context"
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/templates"
)

func runWeb() error {
	fmt.Println("catalog update: not implemented")
	return nil
}

func runLocal(tmpl *templates.Templates) error {
	fmt.Println("Starting Update")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := tmpl.Catalog().Download(ctx); err != nil {
		return fmt.Errorf("download catalog: %w", err)
	}

	fmt.Println("Catalog Updated")

	if err := tmpl.Download(ctx); err != nil {
		return fmt.Errorf("download templates: %w", err)
	}

	fmt.Println("Templates Updated")

	return nil
}
