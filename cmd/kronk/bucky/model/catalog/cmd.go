// Package catalog prints the bundled whisper model catalog.
package catalog

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	"github.com/spf13/cobra"
)

// Cmd is the cobra command for "kronk bucky model catalog".
var Cmd = &cobra.Command{
	Use:   "catalog",
	Short: "List the bundled catalog of well-known whisper models",
	Long: `List the bundled catalog of well-known whisper models that the bucky
backend knows how to download by short name (tiny, base.en, large-v3,
...). Each row reports the short name, the published download size, and
the resolved download URL.

MODES

  Web Mode (default): Reads the bundled catalog through the model
    server at /v1/bucky/models/catalog.
  Local Mode (--local): Reads the bundled catalog in-process without
    contacting a server.

EXAMPLES

  # List every bundled catalog entry from a running server.
  kronk bucky model catalog

  # List the bundled catalog in-process.
  kronk bucky model catalog --local`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")
	if local {
		return runLocal()
	}
	return runWeb()
}

func runWeb() error {
	url, err := client.DefaultURL("/v1/bucky/models/catalog")
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

	var resp toolapp.BuckyCatalogResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return fmt.Errorf("do: unable to get bucky catalog: %w", err)
	}

	if len(resp.Models) == 0 {
		fmt.Println("no catalog entries")
		return nil
	}

	fmt.Printf("%-20s %-10s %s\n", "NAME", "SIZE", "URL")
	for _, e := range resp.Models {
		fmt.Printf("%-20s %-10s %s\n", e.ID, e.Size, e.URL)
	}

	return nil
}

func runLocal() error {
	entries := models.Catalog()
	if len(entries) == 0 {
		fmt.Println("no catalog entries")
		return nil
	}

	names := models.SupportedModels()

	fmt.Printf("%-20s %-10s %s\n", "NAME", "SIZE", "URL")
	for _, name := range names {
		e := entries[name]
		fmt.Printf("%-20s %-10s %s\n", name, e.Size, e.URL)
	}

	return nil
}
