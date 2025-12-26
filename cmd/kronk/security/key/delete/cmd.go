package delete

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a private key",
	Long: `Delete a private key by its key ID (file name without extension)

The master key cannot be deleted.

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.`,
	Args: cobra.NoArgs,
	Run:  main,
}

func init() {
	Cmd.Flags().String("keyid", "", "The key ID to delete (required)")
	Cmd.MarkFlagRequired("keyid")
	Cmd.Flags().Bool("local", false, "Run without the model server")
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	keyID, _ := cmd.Flags().GetString("keyid")
	local, _ := cmd.Flags().GetBool("local")

	var err error

	switch local {
	case true:
		err = runLocal(keyID)
	default:
		err = runWeb(keyID)
	}

	if err != nil {
		return err
	}

	return nil
}
