// Package catalog provide support for the catalog sub-command.
package catalog

import (
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/list"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/pull"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/show"
	"github.com/ardanlabs/kronk/cmd/kronk/catalog/update"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage model catalog",
	Long:  `Manage model catalog - list and update available models`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(list.Cmd)
	Cmd.AddCommand(pull.Cmd)
	Cmd.AddCommand(show.Cmd)
	Cmd.AddCommand(update.Cmd)
}
