package stop

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Kronk model server",
	Long:  `Stop the Kronk model server by sending SIGTERM`,
	Args:  cobra.NoArgs,
	Run:   main,
}

func main(cmd *cobra.Command, args []string) {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	return runLocal()
}
