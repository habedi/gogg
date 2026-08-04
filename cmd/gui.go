//go:build !headless

package cmd

import (
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/gui"
	"github.com/spf13/cobra"
)

func guiCmd(authService *auth.Service, gogClient *client.GogClient) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the Gogg GUI",
		Run: func(cmd *cobra.Command, args []string) {
			gui.Run(version, authService, gogClient)
		},
	}
	return cmd
}
