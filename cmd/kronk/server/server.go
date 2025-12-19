// Package server provide support for the server sub-command.
package server

import (
	"github.com/ardanlabs/kronk/cmd/kronk/server/logs"
	"github.com/ardanlabs/kronk/cmd/kronk/server/start"
	"github.com/ardanlabs/kronk/cmd/kronk/server/stop"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "server",
	Short: "Manage model server",
	Long:  `Manage model server - start, stop, logs`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(start.Cmd)
	Cmd.AddCommand(stop.Cmd)
	Cmd.AddCommand(logs.Cmd)
}
