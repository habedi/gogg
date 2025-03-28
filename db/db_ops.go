package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// Db is the global database connection object
	Db *gorm.DB
	// Path is the default path to the SQLite database file
	Path = filepath.Join(os.Getenv("HOME"), ".gogg/games.db")
)

// InitDB initializes the database by creating the necessary directory,
// opening the database connection, migrating tables, and configuring the logger.
func InitDB() error {
	if err := createDBDirectory(); err != nil {
		return err
	}

	if err := openDatabase(); err != nil {
		return err
	}

	if err := migrateTables(); err != nil {
		return err
	}

	configureLogger()

	log.Info().Msg("Database initialized successfully")
	return nil
}

// createDBDirectory creates the directory for the database file if it does not exist.
func createDBDirectory() error {
	dir := filepath.Dir(Path)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			log.Error().Err(err).Msg("Failed to create database directory")
			return err
		}
	}
	return nil
}

// openDatabase opens a connection to the SQLite database.
func openDatabase() error {
	var err error
	Db, err = gorm.Open(sqlite.Open(Path), &gorm.Config{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize database")
		return err
	}
	return nil
}

// migrateTables performs automatic migration for the Game and Token tables.
func migrateTables() error {
	if err := Db.AutoMigrate(&Game{}, &Token{}); err != nil {
		log.Error().Err(err).Msg("Failed to auto-migrate database")
		return err
	}
	return nil
}

// configureLogger configures the logger for the database based on the global log level.
func configureLogger() {
	if zerolog.GlobalLevel() == zerolog.Disabled {
		Db.Logger = Db.Logger.LogMode(0)
	} else {
		Db.Logger = Db.Logger.LogMode(4)
	}
}

// CloseDB closes the database connection.
func CloseDB() error {
	sqlDB, err := Db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get raw database connection")
		return err
	}
	return sqlDB.Close()
}

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
			log.Error().Err(err).Msgf("Failed to insert new token: %s", token.AccessToken[:10])
			return err
		}
		log.Info().Msgf("Token inserted successfully: %s", token.AccessToken[:10])
	} else {
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
