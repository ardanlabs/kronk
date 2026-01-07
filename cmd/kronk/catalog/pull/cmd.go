package pull

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "pull <MODEL_ID>",
	Short: "Pull a model from the catalog",
	Long: `Pull a model from the catalog

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.`,
	Args: cobra.ExactArgs(1),
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd, args); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	local, _ := cmd.Flags().GetBool("local")

	catalog, err := catalog.NewWithSettings("", "")
	if err != nil {
		return fmt.Errorf("unable to create catalog system: %w", err)
	}

	if err := catalog.Download(context.Background()); err != nil {
		return fmt.Errorf("unable to download catalog: %w", err)
	}

	models, err := models.NewWithPaths("")
	if err != nil {
		return fmt.Errorf("unable to create models system: %w", err)
	}

	switch local {
	case true:
		err = runLocal(catalog, models, args)
	default:
		err = runWeb(args)
	}

	if err != nil {
		return err
	}

	return nil
}
