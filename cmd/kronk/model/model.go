// Package model provide support for the model sub-command.
package model

import (
	"github.com/ardanlabs/kronk/cmd/kronk/model/list"
	"github.com/ardanlabs/kronk/cmd/kronk/model/ps"
	"github.com/ardanlabs/kronk/cmd/kronk/model/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/model/remove"
	"github.com/ardanlabs/kronk/cmd/kronk/model/show"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models",
	Long:  `Manage models - list, pull, remove, show, and check running models`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(pull.Cmd)
	Cmd.AddCommand(remove.Cmd)
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(ps.Cmd)
}
