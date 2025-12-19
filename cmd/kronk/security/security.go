// Package security provides tooling support for security.
package security

import (
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/security/key"
	"github.com/ardanlabs/kronk/cmd/kronk/security/token"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "security",
	Short: "Manage security",
	Long: `Manage security - tokens and access control

Environment Variables:
  KRONK_TOKEN    Admin level token required for authentication. Must be set
                 before running any security commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(key.Cmd)
	Cmd.AddCommand(token.Cmd)

	if os.Getenv("KRONK_TOKEN") == "" {
		Cmd.Help()
		os.Exit(0)
	}
}
