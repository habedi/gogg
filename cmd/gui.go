package cmd

import (
	"github.com/habedi/gogg/ui" // update the import path as needed
	"github.com/spf13/cobra"
)

func guiCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the Gogg GUI",
		Run: func(cmd *cobra.Command, args []string) {
			ui.Run()
		},
	}
	return cmd
}
