package cmd

import (
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// See https://gogapidocs.readthedocs.io/en/latest/auth.html for more information on GOG's authentication API

var (
	// authURL is the URL to authenticate with GOG.com
	authURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
		"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
		"&response_type=code&layout=client2"
)

// authCmd authenticates the user with GOG.com and retrieves an access token or renew the token
func authCmd() *cobra.Command {
	var showWindow bool

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate user account using the GOG API",
		Run: func(cmd *cobra.Command, args []string) {
			log.Info().Msg("Trying to authenticate with GOG.com")

			_, err := authenticateUser(showWindow)
			if err != nil {
				cmd.PrintErrln("Error: Failed to authenticate. Please make sure you have run 'gogg init', check your credentials, and try again.")
				return
			}

			cmd.Println("Authentication was successful.")
		},
	}

	cmd.Flags().BoolVarP(&showWindow, "show", "s", true, "Show the login window in the browser [true, false]")

	return cmd
}
