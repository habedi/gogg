package cmd

import (
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/gui"
	"github.com/spf13/cobra"
)

func guiCmd(authService *auth.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the Gogg GUI",
		Run: func(cmd *cobra.Command, args []string) {
			gui.Run(version, authService)
		},
	}
	return cmd
}
