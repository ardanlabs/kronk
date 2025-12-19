package logs

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream server logs",
	Long:  `Stream the Kronk model server logs (tail -f)`,
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
