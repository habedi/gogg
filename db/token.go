package db

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Token represents the persisted auth tokens. Currently, we enforce a single-row table
// by always upserting record with ID=1.
type Token struct {
	ID           uint   `gorm:"primaryKey"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresAt    string `json:"expires_at,omitempty"`
}

// TokenStore is a concrete implementation of the auth.TokenStorer interface using GORM.
// Deprecated: prefer using the TokenRepository interface for new code.
type TokenStore struct{}

func (ts *TokenStore) GetTokenRecord() (*Token, error) {
	return GetTokenRecord()
}

func (ts *TokenStore) UpsertTokenRecord(token *Token) error {
	return UpsertTokenRecord(token)
}

func GetTokenRecord() (*Token, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var token Token
	if err := Db.First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.Error().Err(err).Msg("Failed to retrieve token data")
		return nil, err
	}

	return &token, nil
}

func UpsertTokenRecord(token *Token) error {
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	token.ID = 1

	if err := Db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"access_token", "refresh_token", "expires_at"}),
	}).Create(token).Error; err != nil {
		log.Error().Err(err).Msgf("Failed to upsert token")
		return err
	}

	log.Info().Msgf("Token upserted successfully")
	return nil
}
