package cmd

import (
	"runtime"

	"github.com/spf13/cobra"
)

var (
	version   = "0.4.3-beta"
	goVersion = runtime.Version()
	platform  = runtime.GOOS + "/" + runtime.GOARCH
)

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
