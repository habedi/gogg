package cmd

import (
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// authenticateUser handles the authentication process
func authenticateUser(showWindow bool) (*db.User, error) {
	user, err := db.GetUserData()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return nil, err
	}

	if err := client.AuthGOG(authURL, user, !showWindow); err != nil {
		log.Error().Err(err).Msg("Failed to authenticate with GOG.com")
		return nil, err
	}

	return user, nil
}
