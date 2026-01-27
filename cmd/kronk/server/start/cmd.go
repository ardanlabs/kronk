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

	// Web settings
	Cmd.Flags().String("api-host", "", "API host address (e.g., localhost:8080)")
	Cmd.Flags().String("debug-host", "", "Debug host address (e.g., localhost:8090)")
	Cmd.Flags().String("read-timeout", "", "HTTP read timeout (e.g., 30s)")
	Cmd.Flags().String("write-timeout", "", "HTTP write timeout (e.g., 15m)")
	Cmd.Flags().String("idle-timeout", "", "HTTP idle timeout (e.g., 1m)")
	Cmd.Flags().String("shutdown-timeout", "", "Server shutdown timeout (e.g., 1m)")
	Cmd.Flags().StringSlice("cors-allowed-origins", nil, "CORS allowed origins")

	// Auth settings
	Cmd.Flags().Bool("auth-enabled", false, "Enable local authentication")
	Cmd.Flags().String("auth-host", "", "External auth service host")
	Cmd.Flags().String("auth-issuer", "", "Local auth issuer name")

	// Tempo/tracing settings
	Cmd.Flags().String("tempo-host", "", "Tempo host address (e.g., localhost:4317)")
	Cmd.Flags().String("tempo-service-name", "", "Tempo service name")
	Cmd.Flags().Float64("tempo-probability", -1, "Tempo sampling probability (0.0-1.0)")

	// Catalog settings
	Cmd.Flags().String("catalog-github-repo", "", "GitHub repo URL for catalogs")
	Cmd.Flags().String("model-config-file", "", "Special config file for model specific config")

	// Templates settings
	Cmd.Flags().String("templates-github-repo", "", "GitHub repo URL for templates")

	// Cache settings
	Cmd.Flags().Int("model-instances", 0, "Maximum model instances")
	Cmd.Flags().Int("models-in-cache", 0, "Maximum models in cache")
	Cmd.Flags().String("cache-ttl", "", "Cache TTL duration (e.g., 5m, 1h)")
	Cmd.Flags().Bool("ignore-integrity-check", false, "Ignore model integrity check")

	// Runtime settings
	Cmd.Flags().String("base-path", "", "Base path for kronk data")
	Cmd.Flags().String("lib-path", "", "Path to llama library")
	Cmd.Flags().String("lib-version", "", "Version of llama library")
	Cmd.Flags().String("arch", "", "Architecture override")
	Cmd.Flags().String("os", "", "OS override")
	Cmd.Flags().String("processor", "", "Processor type (e.g., vulkan, metal, cuda)")
	Cmd.Flags().String("hf-token", "", "Hugging Face API token")
	Cmd.Flags().Bool("allow-upgrade", true, "Allow automatic upgrades")
	Cmd.Flags().Int("llama-log", -1, "Llama log level (0=off, 1=on)")
	Cmd.Flags().String("device", "", "Device to use for inference (e.g., cuda, metal)")

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
