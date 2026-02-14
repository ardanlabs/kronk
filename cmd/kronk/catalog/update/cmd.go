package update

import (
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "update",
	Short: "Update the model catalog",
	Long: `Update the model catalog

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.

Environment Variables (--local mode):
      KRONK_BASE_PATH  Base path for kronk data (models, templates, catalog)`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	local, _ := cmd.Flags().GetBool("local")

	ctlg, err := catalog.New(catalog.WithBasePath(client.GetBasePath(cmd)))
	if err != nil {
		return fmt.Errorf("unable to create catalog system: %w", err)
	}

	switch local {
	case true:
		err = runLocal(ctlg)
	default:
		err = runWeb()
	}

	if err != nil {
		return err
	}

	return nil
}
