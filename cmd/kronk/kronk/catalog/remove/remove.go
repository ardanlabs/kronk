// Package remove provides the catalog remove command code.
package remove

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

func runWeb(args []string) error {
	id := args[0]

	url, err := client.DefaultURL(fmt.Sprintf("/v1/catalog/%s", id))
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	if !confirm(id) {
		return nil
	}

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := cln.Do(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("remove-catalog: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}

func runLocal(mdls *models.Models, args []string) error {
	id := args[0]

	fmt.Println("Catalog ID:", id)

	if !confirm(id) {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := mdls.RemoveCatalogEntry(ctx, id, kronk.FmtLogger); err != nil {
		return fmt.Errorf("remove-catalog: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}

// =============================================================================

func confirm(id string) bool {
	fmt.Printf("\nAre you sure you want to remove %q? This deletes the catalog entry, GGUF cache, and any downloaded files. (y/n): ", id)

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return false
	}

	return true
}
