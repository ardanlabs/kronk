package list

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog models",
	Long: `List catalog models

Flags (--local mode):
      --filter-category  Filter catalogs by category name (substring match)

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_BASE_PATH  Base path for kronk data (models, templates, catalog)`,
	Args: cobra.ArbitraryArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
	Cmd.Flags().String("filter-category", "", "Filter catalogs by category name (substring match)")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")
	filterCategory, _ := cmd.Flags().GetString("filter-category")

	if filterCategory != "" {
		args = append(args, "--filter-category", filterCategory)
	}

	catalog, err := catalog.New(catalog.WithBasePath(client.GetBasePath(cmd)))
	if err != nil {
		return fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := catalog.Download(context.Background()); err != nil {
		return fmt.Errorf("unable to download catalog: %w", err)
	}

	switch local {
	case true:
		err = runLocal(catalog, args)
	default:
		err = runWeb()
	}

	if err != nil {
		return err
	}

	return nil
}
