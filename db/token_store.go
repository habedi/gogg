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

	return &token, nil
}

// UpsertTokenRecord inserts or updates the token record in the database.
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
		if err := Db.Create(token).Error; err != nil {
			log.Error().Err(err).Msg("Failed to insert new token")
			return err
		}
		log.Info().Msg("Token inserted successfully")
	} else {
		if err := Db.Model(&existingToken).Where("1 = 1").Updates(Token{
			AccessToken:  token.AccessToken,
			RefreshToken: token.RefreshToken,
			ExpiresAt:    token.ExpiresAt,
		}).Error; err != nil {
			log.Error().Err(err).Msg("Failed to update token")
			return err
		}
		log.Info().Msg("Token updated successfully")
	}

	return nil
}
