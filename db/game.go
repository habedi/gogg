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
// It takes the game ID, title, and data as parameters and returns an error if the operation fails.
func PutInGame(id int, title, data string) error {
	game := Game{
		ID:    id,
		Title: title,
		Data:  data,
	}

	return upsertGame(game)
}

// upsertGame performs an upsert operation on the game record.
// It takes a Game object as a parameter and returns an error if the operation fails.
func upsertGame(game Game) error {
	if err := Db.Clauses(
		clause.OnConflict{
			UpdateAll: true, // Updates all fields if there's a conflict on the primary key (ID).
		},
	).Create(&game).Error; err != nil {
		log.Error().Err(err).Msgf("Failed to upsert game with ID %d", game.ID)
		return err
	}

	log.Info().Msgf("Game upserted successfully: ID=%d, Title=%s", game.ID, game.Title)
	return nil
}

// EmptyCatalogue removes all records from the game catalogue.
// It returns an error if the operation fails.
func EmptyCatalogue() error {
	if err := Db.Unscoped().Where("1 = 1").Delete(&Game{}).Error; err != nil {
		log.Error().Err(err).Msg("Failed to empty game catalogue")
		return err
	}

	log.Info().Msg("Game catalogue emptied successfully")
	return nil
}

// GetCatalogue retrieves all games in the catalogue.
// It returns a slice of Game objects and an error if the operation fails.
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
// It takes the game ID as a parameter and returns a pointer to the Game object and an error if the operation fails.
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
// It takes the game name as a parameter and returns a slice of Game objects and an error if the operation fails.
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
