package start

import (
	"fmt"
	"os"

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
	Cmd.Flags().BoolP("detach", "d", false, "Run server in the background")

	Cmd.Flags().String("api-host", "", "API host address (e.g., localhost:8080)")
	Cmd.Flags().String("debug-host", "", "Debug host address (e.g., localhost:8090)")
	Cmd.Flags().Bool("auth-enabled", false, "Enable local authentication")
	Cmd.Flags().String("device", "", "Device to use for inference (e.g., cuda, metal)")
	Cmd.Flags().Int("max-instances", 0, "Maximum model instances")
	Cmd.Flags().Int("max-in-cache", 0, "Maximum models in cache")
	Cmd.Flags().Int("context-window", 0, "Context window size")
	Cmd.Flags().String("cache-ttl", "", "Cache TTL duration (e.g., 5m, 1h)")
	Cmd.Flags().Int("llama-log", -1, "Llama log level (0=off, 1=on)")

	Cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		err := kronk.Run(true)
		cmd.Long = fmt.Sprintf("Start Kronk model server\n\n%s", err.Error())
		cmd.Parent().HelpFunc()(cmd, args)
	})
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
