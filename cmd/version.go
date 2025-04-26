package cmd

import (
	"github.com/spf13/cobra"
)

var version = "0.5.0-beta"

func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Gogg version:", version)
		},
	}
	return cmd
}
