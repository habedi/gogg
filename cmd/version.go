package cmd

import (
	"github.com/spf13/cobra"
	"runtime"
)

var version = "0.4.1-beta"
var goVersion = runtime.Version()
var platform = runtime.GOOS + "/" + runtime.GOARCH

func versionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Println("Gogg version:", version)
			cmd.Println("Go version:", goVersion)
			cmd.Println("Platform:", platform)
		},
	}
	return cmd
}
