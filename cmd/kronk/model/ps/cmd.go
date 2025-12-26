package ps

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "ps",
	Short: "List running models",
	Long: `List running models

Environment Variables:
      KRONK_TOKEN         (required when auth enabled)  Authentication token for the kronk server.
      KRONK_WEB_API_HOST  (default localhost:8080)  IP Address for the kronk server.`,
	Run: main,
}

func main(cmd *cobra.Command, args []string) {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	if err := runWeb(); err != nil {
		return err
	}

	return nil
}
