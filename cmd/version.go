package cmd

import (
	"github.com/spf13/cobra"
)

// version holds the current version of the Gogg.
var version = "0.5.0-beta"

// versionCmd creates a new cobra.Command that shows the version of Gogg.
// It returns a pointer to the created cobra.Command.
func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			// Print the current version of Gogg to the command line.
			cmd.Println("Gogg version:", version)
		},
	}
	return cmd
}
