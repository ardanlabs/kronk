package index

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "index",
	Short: "Rebuild the model index",
	Long: `Rebuild the model index for fast model access

Environment Variables (web mode - default):
      KRONK_WEB_API_HOST  (default localhost:3000)  IP Address for the kronk server

Environment Variables (--local mode):
      KRONK_MODELS  (default: $HOME/kronk/models)  The path to the models directory`,
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
