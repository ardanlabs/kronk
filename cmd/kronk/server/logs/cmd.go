package logs

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream server logs",
	Long:  `Stream the Kronk model server logs`,
	Args:  cobra.NoArgs,
	Run:   main,
}

func main(cmd *cobra.Command, args []string) {
	if err := run(cmd); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command) error {
	return runLocal(cmd)
}
