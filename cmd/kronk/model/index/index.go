// Package index provides the index command code.
package index

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/sdk/client"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/models/index")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := client.Do(ctx, http.MethodPost, url, nil, nil); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	fmt.Println("Index rebuilt successfully")

	return nil
}

func runLocal(models *models.Models) error {
	fmt.Println("Model Path:", models.Path())

	if err := models.BuildIndex(); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	fmt.Println("Index rebuilt successfully")

	return nil
}
