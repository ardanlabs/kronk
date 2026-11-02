package start

import (
	"context"
	"fmt"

	"github.com/ardanlabs/kronk/cmd/kronk/service"
	"github.com/ardanlabs/kronk/cmd/server/api/services/kronk"
	"github.com/spf13/cobra"
)

var Cmd = NewCommand()

// NewCommand constructs the server command used by interactive and managed starts.
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start Kronk model server",
		Long:  `Start Kronk model server. Use --help to get environment settings.`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return service.Run(cmd, func(ctx context.Context, ready func()) error {
				cmd.SetContext(ctx)
				return runLocal(cmd, ready)
			})
		},
	}
	configure(cmd)
	return cmd
}

func configure(cmd *cobra.Command) {
	cmd.Flags().BoolP("detach", "d", false, "Run server in the background")

	// Web settings
	cmd.Flags().String("api-host", "", "API host address (e.g., localhost:11435)")
	cmd.Flags().String("debug-host", "", "Debug host address (e.g., localhost:11445)")
	cmd.Flags().String("read-timeout", "", "HTTP read timeout (e.g., 30s)")
	cmd.Flags().String("inference-timeout", "", "Total inference timeout (e.g., 60m)")
	cmd.Flags().String("write-timeout", "", "HTTP write timeout (e.g., 15m)")
	cmd.Flags().String("idle-timeout", "", "HTTP idle timeout (e.g., 1m)")
	cmd.Flags().String("shutdown-timeout", "", "Server shutdown timeout (e.g., 1m)")
	cmd.Flags().StringSlice("cors-allowed-origins", nil, "CORS allowed origins")

	// Auth settings
	cmd.Flags().Bool("web-admin-enabled", false, "Serve the browser admin UI at /admin/")
	cmd.Flags().String("auth-host", "", "External auth service host")
	cmd.Flags().Bool("auth-tls-enabled", false, "Use TLS for the external auth service")
	cmd.Flags().String("auth-tls-ca-file", "", "CA certificate file for the external auth service")
	cmd.Flags().String("auth-tls-server-name", "", "TLS server name for the external auth service")
	cmd.Flags().String("auth-issuer", "", "Local auth issuer name")
	cmd.Flags().String("web-admin-password-sha-256", "", "SHA-256 digest for browser admin login")
	cmd.Flags().String("web-admin-session-duration", "", "Browser admin session lifetime (e.g., 12h; default: 12h)")
	cmd.Flags().String("authorization-mode", "", "API authorization mode (open, management, authenticated, full-protected)")

	// MCP settings
	cmd.Flags().Bool("mcp-enabled", true, "Enable or disable the embedded MCP service")
	cmd.Flags().String("mcp-host", "", "Disable the embedded MCP service when an external host is configured")
	cmd.Flags().Bool("mcp-auth-enabled", false, "Require a Kronk admin bearer token for MCP access")
	cmd.Flags().String("mcp-brave-api-key", "", "Brave Search API key for the embedded MCP service")
	cmd.Flags().Bool("download-enabled", false, "Allow server-side model downloads")

	// Tempo/tracing settings
	cmd.Flags().String("tempo-host", "", "Tempo host address (e.g., localhost:4317)")
	cmd.Flags().String("tempo-service-name", "", "Tempo service name")
	cmd.Flags().Float64("tempo-probability", -1, "Tempo sampling probability (0.0-1.0)")

	// Catalog settings
	cmd.Flags().String("model-config-file", "", "Special config file for model specific config")

	// Cache settings
	cmd.Flags().Int("budget-percent", 0, "Percentage (1..100) of system/VRAM memory the resource manager may consume (default: 90)")
	cmd.Flags().Int("models-in-pool", 0, "Safety-net cap on the number of distinct models kept loaded, regardless of budget (default: 10)")
	cmd.Flags().String("pool-ttl", "", "Idle model TTL (e.g., 5m, 1h; 0 disables expiration)")

	// Runtime settings
	cmd.Flags().String("base-path", "", "Base path for kronk data")
	cmd.Flags().String("lib-path", "", "Path to llama library")
	cmd.Flags().String("bucky-lib-path", "", "Path to whisper library")
	cmd.Flags().String("malina-lib-path", "", "Path to stable-diffusion library")
	cmd.Flags().String("lib-version", "", "Version of llama library, optionally VERSION@sha256:DIGEST")
	cmd.Flags().Bool("lib-download-enabled", true, "Enable automatic llama library downloads at startup")
	cmd.Flags().Bool("media-backends-enabled", true, "Download and load the Bucky and Malina backends at startup")
	cmd.Flags().Bool("lib-verify-enabled", true, "Verify the selected llama and whisper libraries before loading them")
	cmd.Flags().String("arch", "", "Architecture override")
	cmd.Flags().String("os", "", "OS override")
	cmd.Flags().String("processor", "", "Processor type (e.g., vulkan, metal, cuda, rocm)")
	cmd.Flags().String("hf-token", "", "Hugging Face API token")
	cmd.Flags().Bool("allow-upgrade", false, "Allow automatic upgrades")
	cmd.Flags().Int("llama-log", -1, "Llama log level (0=off, 1=on)")
	cmd.Flags().Bool("insecure-logging", false, "Enable logging of sensitive data (messages, model config)")

	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		err := kronk.Run(cmd.Context(), true, nil)
		cmd.Long = fmt.Sprintf("Start Kronk model server\n\n%s", err.Error())
		cmd.Parent().HelpFunc()(cmd, args)
	})
}
