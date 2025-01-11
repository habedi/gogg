package cmd

import (
	"github.com/spf13/cobra"
)

var (
	// Gogg version
	version = "1.0.0"
)

// versionCmd shows the version of Gogg
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
