package db

import (
	"errors"
	"fmt"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// User holds the user data.
type User struct {
	Username     string `gorm:"primaryKey" json:"username"`
	Password     string `json:"password"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// GetUserData retrieves the user data from the database.
func GetUserData() (*User, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var user User
	if err := Db.First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // User not found
		}
		log.Error().Err(err).Msg("Failed to retrieve user data")
		return nil, err
	}

	if &user == nil {
		return nil, fmt.Errorf("no user data found. Please run 'gogg init' to enter your username and password")
	}

	return &user, nil
}

// UpsertUserData inserts or updates the user data in the database.
func UpsertUserData(user *User) error {
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	if err := Db.Clauses(
		clause.OnConflict{
			UpdateAll: true, // Updates all fields if there's a conflict on the primary key (Username).
		},
	).Create(user).Error; err != nil {
		log.Error().Err(err).Msgf("Failed to upsert user %s", user.Username)
		return err
	}

	log.Info().Msgf("User upserted successfully: %s", user.Username)
	return nil
}
