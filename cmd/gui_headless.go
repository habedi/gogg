//go:build headless

package cmd

import (
	"fmt"

	"github.com/habedi/gogg/auth"
	"github.com/spf13/cobra"
)

func guiCmd(authService *auth.Service) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gui",
		Short: "Start the Gogg GUI (not available in headless build)",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Error: GUI is not available in this build.")
			fmt.Println("This is a headless (CLI-only) version of Gogg.")
			fmt.Println("To use the GUI, please download the full version for your platform")
			fmt.Println("or build from source without the 'headless' tag.")
		},
	}
	return cmd
}
