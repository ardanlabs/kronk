package logs

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream server logs",
	Long:  `Stream logs from a server started with --detach. For an OS-managed service, use "kronk systemd logs" on Linux or "kronk launchd logs" on macOS.`,
	Args:  cobra.NoArgs,
	RunE:  main,
}

func main(cmd *cobra.Command, args []string) error {
	return run(cmd)
}

func run(cmd *cobra.Command) error {
	return runLocal(cmd)
}
