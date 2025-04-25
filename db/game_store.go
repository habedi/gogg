package db

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Game represents a game record in the catalogue.
type Game struct {
	ID    int    `gorm:"primaryKey" json:"id"`
	Title string `gorm:"index" json:"title"` // Indexed for faster queries
	Data  string `json:"data"`
}

// PutInGame inserts or updates a game record in the catalogue.
func PutInGame(id int, title, data string) error {
	game := Game{
		ID:    id,
		Title: title,
		Data:  data,
	}
	return upsertGame(game)
}

// upsertGame performs an upsert operation on the game record.
func upsertGame(game Game) error {
	if err := Db.Clauses(
		clause.OnConflict{UpdateAll: true},
	).Create(&game).Error; err != nil {
		log.Error().Err(err).Msgf("Failed to upsert game with ID %d", game.ID)
		return err
	}

	log.Info().Msgf("Game upserted successfully: ID=%d, Title=%s", game.ID, game.Title)
	return nil
}

// EmptyCatalogue removes all records from the game catalogue.
func EmptyCatalogue() error {
	if err := Db.Unscoped().Where("1 = 1").Delete(&Game{}).Error; err != nil {
		log.Error().Err(err).Msg("Failed to empty game catalogue")
		return err
	}

	log.Info().Msg("Game catalogue emptied successfully")
	return nil
}

// GetCatalogue retrieves all games in the catalogue.
func GetCatalogue() ([]Game, error) {
	var games []Game
	if err := Db.Find(&games).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch games from the database")
		return nil, err
	}

	log.Info().Msgf("Retrieved %d games from the catalogue", len(games))
	return games, nil
}

// GetGameByID retrieves a game from the catalogue by its ID.
func GetGameByID(id int) (*Game, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var game Game
	if err := Db.First(&game, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Game not found
		}
		return nil, fmt.Errorf("failed to retrieve game with ID %d: %w", id, err)
	}

	return &game, nil
}

// SearchGamesByName searches for games in the catalogue by name.
func SearchGamesByName(name string) ([]Game, error) {
	if Db == nil {
		return nil, fmt.Errorf("database connection is not initialized")
	}

	var games []Game
	if err := Db.Where("title LIKE ?", "%"+name+"%").Find(&games).Error; err != nil {
		log.Error().Err(err).Msgf("Failed to search games by name: %s", name)
		return nil, err
	}

	return games, nil
}
