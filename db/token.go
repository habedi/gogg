package db

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Token represents the user's authentication token data.
type Token struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// GetTokenRecord retrieves the token record from the database.
// It returns a pointer to the Token object and an error if the operation fails.
func GetTokenRecord() (*Token, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var token Token
	if err := Db.First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Token not found
		}
		log.Error().Err(err).Msg("Failed to retrieve token data")
		return nil, err
	}

	//if &token == nil {
	//	return nil, fmt.Errorf("no token data found. Please try logging in first")
	//}

	return &token, nil
}

// UpsertTokenRecord inserts or updates the token record in the database.
// It takes a pointer to the Token object as a parameter and returns an error if the operation fails.
func UpsertTokenRecord(token *Token) error {
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	var existingToken Token
	err := Db.First(&existingToken).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Error().Err(err).Msg("Failed to check existing token")
		return err
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Insert new token
		if err := Db.Create(token).Error; err != nil {
			log.Error().Err(err).Msgf("Failed to insert new token: %s", token.AccessToken[:10])
			return err
		}
		log.Info().Msgf("Token inserted successfully: %s", token.AccessToken[:10])
	} else {
		// Update existing token
		if err := Db.Model(&existingToken).Where("1 = 1").Updates(Token{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			ExpiresAt:    token.ExpiresAt,
		}).Error; err != nil {
			log.Error().Err(err).Msgf("Failed to update token: %s", token.AccessToken[:10])
			return err
		}
		log.Info().Msgf("Token updated successfully: %s", token.AccessToken[:10])
	}

	return nil
}
