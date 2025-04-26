package cmd

import (
	"github.com/habedi/gogg/gui"
	"github.com/spf13/cobra"
)

func guiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the Gogg GUI",
		Run: func(cmd *cobra.Command, args []string) {
			gui.Run(version)
		},
	}
	return cmd
}
