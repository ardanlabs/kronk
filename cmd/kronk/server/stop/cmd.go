package stop

import (
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the Kronk model server",
	Long:  `Stop a server started with --detach by sending SIGTERM. For an OS-managed service, use "kronk systemd stop" on Linux or "kronk launchd stop" on macOS.`,
	Args:  cobra.NoArgs,
	RunE:  main,
}

func main(cmd *cobra.Command, args []string) error {
	return run(cmd)
}

func run(cmd *cobra.Command) error {
	return runLocal(cmd)
}
