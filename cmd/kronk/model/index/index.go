// Package index provides the index command code.
package index

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/models/index")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := cln.Do(ctx, http.MethodPost, url, nil, nil); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	fmt.Println("Index rebuilt successfully")

	return nil
}

func runLocal(models *models.Models) error {
	fmt.Println("Model Path:", models.Path())

	if err := models.BuildIndex(kronk.FmtLogger); err != nil {
		return fmt.Errorf("build-index: %w", err)
	}

	fmt.Println("Index rebuilt successfully")

	return nil
}
