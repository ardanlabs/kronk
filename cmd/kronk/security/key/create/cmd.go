package create

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new private key",
	Long: `Create a new private key and add it to the keystore

Environment Variables (web mode - default):
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.`,
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

	var err error

	switch local {
	case true:
		err = runLocal()
	default:
		err = runWeb()
	}

	if err != nil {
		return err
	}

	return nil
}
