// Package key provides tooling support for security keys.
package key

import (
	"github.com/ardanlabs/kronk/cmd/kronk/security/key/create"
	"github.com/ardanlabs/kronk/cmd/kronk/security/key/delete"
	"github.com/ardanlabs/kronk/cmd/kronk/security/key/list"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "key",
	Short: "Manage private keys",
	Long:  `Manage private keys - create and delete private keys`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(create.Cmd)
	Cmd.AddCommand(delete.Cmd)
	Cmd.AddCommand(list.Cmd)
}
