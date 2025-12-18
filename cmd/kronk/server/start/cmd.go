package start

import (
	"fmt"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "start",
	Short: "Start Kronk model server",
	Long:  `Start Kronk model server. Use --help to get environment settings`,
	Args:  cobra.NoArgs,
	Run:   main,
}

func init() {
	if len(os.Args) > 1 {
		v := strings.Join(os.Args[1:], " ")
		if v == "server start --help" || v == "help server start" {
			err := kronk.Run(true)
			Cmd = &cobra.Command{
				Use:   "start",
				Short: "Start kronk server",
				Long:  fmt.Sprintf("Start kronk server\n\n%s", err.Error()),
				Args:  cobra.NoArgs,
				Run:   main,
			}
		}
	}

	Cmd.Flags().BoolP("detach", "d", false, "Run server in the background")
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
