// Package token provides tooling support for security tokens.
package token

import (
	"github.com/ardanlabs/kronk/cmd/kronk/security/token/create"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "token",
	Short: "Manage tokens",
	Long:  `Manage tokens - create and manage security tokens`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(create.Cmd)
}
