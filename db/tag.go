package db

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm/clause"
)

// Tag names gogg gives meaning to itself. Anything else is the user's own.
const (
	// TagFavorite marks a game the user wants near the top.
	TagFavorite = "favorite"
	// TagHidden marks a game the user does not want listed.
	TagHidden = "hidden"
)

// GameTag marks a game with a word the user chose. It is a table of its own
// rather than a column so a game can carry any number of them, and so a
// catalogue refresh never has to know about them.
type GameTag struct {
	GameID int    `gorm:"primaryKey" json:"game_id"`
	Tag    string `gorm:"primaryKey" json:"tag"`
}

// AddTag marks a game. Marking it twice is not an error.
func AddTag(ctx context.Context, gameID int, tag string) error {
	tag = normalizeTag(tag)
	if tag == "" {
		return fmt.Errorf("a tag cannot be empty")
	}
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	err := Db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).
		Create(&GameTag{GameID: gameID, Tag: tag}).Error
	if err != nil {
		return fmt.Errorf("failed to tag game %d: %w", gameID, err)
	}
	return nil
}

// RemoveTag unmarks a game. Unmarking one that was not marked is not an error.
func RemoveTag(ctx context.Context, gameID int, tag string) error {
	if Db == nil {
		return fmt.Errorf("database connection is not initialized")
	}

	err := Db.WithContext(ctx).
		Where("game_id = ? AND tag = ?", gameID, normalizeTag(tag)).
		Delete(&GameTag{}).Error
	if err != nil {
		return fmt.Errorf("failed to untag game %d: %w", gameID, err)
	}
	return nil
}

// AllTags returns every tag in the catalogue, keyed by game. The library asks
// for the lot at once: a query is run against every game as the user types.
func AllTags(ctx context.Context) (map[int][]string, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var rows []GameTag
	if err := Db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read tags: %w", err)
	}

	tags := make(map[int][]string, len(rows))
	for _, row := range rows {
		tags[row.GameID] = append(tags[row.GameID], row.Tag)
	}
	return tags, nil
}

// TagsFor returns what one game is marked with.
func TagsFor(ctx context.Context, gameID int) ([]string, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var rows []GameTag
	if err := Db.WithContext(ctx).Where("game_id = ?", gameID).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to read tags for game %d: %w", gameID, err)
	}

	tags := make([]string, 0, len(rows))
	for _, row := range rows {
		tags = append(tags, row.Tag)
	}
	return tags, nil
}

// normalizeTag settles on one spelling, so a game marked "Favorite" is not a
// different thing from one marked "favorite".
func normalizeTag(tag string) string {
	return strings.ToLower(strings.TrimSpace(tag))
}
