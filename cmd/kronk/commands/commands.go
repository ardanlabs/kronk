// Package commands owns the command tree shared by the CLI and its reference generator.
package commands

import (
	"github.com/ardanlabs/kronk/cmd/kronk/bucky"
	"github.com/ardanlabs/kronk/cmd/kronk/devices"
	"github.com/ardanlabs/kronk/cmd/kronk/diagnose"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/catalog"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/libs"
	"github.com/ardanlabs/kronk/cmd/kronk/kronk/model"
	"github.com/ardanlabs/kronk/cmd/kronk/launch"
	"github.com/ardanlabs/kronk/cmd/kronk/launchd"
	"github.com/ardanlabs/kronk/cmd/kronk/malina"
	"github.com/ardanlabs/kronk/cmd/kronk/security"
	"github.com/ardanlabs/kronk/cmd/kronk/server"
	"github.com/ardanlabs/kronk/cmd/kronk/systemd"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/spf13/cobra"
)

// Cmd is the root command. Constructing the tree does not execute any operations.
var Cmd = newCommand()

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "kronk",
		Short:   "Local LLM inference with hardware acceleration",
		Version: kronk.Version,
		Long: `Kronk runs local inference with hardware acceleration and provides tools
for provisioning models, managing libraries and credentials, and operating the service.

Management commands use a running server by default. Commands offering --local
can operate directly on files without a server. See each command's help for details.

See the manual for installation, environment settings, security, and service workflows.`,
		Example: `  # Download a model without a running server
  kronk model pull unsloth/Qwen3-1.7B-UD-Q8_K_XL --local

  # Start the server with the Browser UI
  kronk server start --web-admin-enabled

  # Open http://localhost:11435/admin/ in your browser`,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	cmd.PersistentFlags().String("base-path", "", "Base path for kronk data (models, libraries, catalog, model_config)")
	cmd.AddCommand(libs.Cmd, model.Cmd, catalog.Cmd, bucky.Cmd, malina.Cmd)
	cmd.AddCommand(server.Cmd, systemd.Cmd, launchd.Cmd, security.Cmd, devices.Cmd, diagnose.Cmd, launch.Cmd)
	return cmd
}
