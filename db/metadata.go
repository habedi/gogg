package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GameMetadataRecord is what GOG's store said about a game, kept with the
// catalogue so one database carries everything known about a library. The
// payload is opaque here: its shape belongs to whoever wrote it, and Format
// names which shape that was.
type GameMetadataRecord struct {
	GameID    int       `gorm:"primaryKey" json:"game_id"`
	Format    int       `json:"format"`
	Data      []byte    `json:"data"`
	FetchedAt time.Time `json:"fetched_at"`
}

// PutGameMetadata records a store lookup, replacing what an earlier one wrote.
func PutGameMetadata(ctx context.Context, gameID, format int, data []byte) error {
	return PutGameMetadataAt(ctx, gameID, format, data, time.Now())
}

// PutGameMetadataAt is PutGameMetadata with the fetch time named by the
// caller, for recording a lookup as older than the moment it is written.
func PutGameMetadataAt(ctx context.Context, gameID, format int, data []byte, fetchedAt time.Time) error {
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	record := GameMetadataRecord{GameID: gameID, Format: format, Data: data, FetchedAt: fetchedAt}
	err := Db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&record).Error
	if err != nil {
		return fmt.Errorf("failed to store metadata for game %d: %w", gameID, err)
	}
	return nil
}

// GetGameMetadata returns what was stored for a game, or nil when nothing was.
func GetGameMetadata(ctx context.Context, gameID int) (*GameMetadataRecord, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var record GameMetadataRecord
	err := Db.WithContext(ctx).First(&record, "game_id = ?", gameID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read metadata for game %d: %w", gameID, err)
	}
	return &record, nil
}
