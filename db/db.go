package db

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Database variables
var (
	Db   *gorm.DB                                             // GORM database instance
	Path = filepath.Join(os.Getenv("HOME"), ".gogg/games.db") // Default database path
)

// InitDB initializes the database and creates the tables if they don't exist.
// It returns an error if any step in the initialization process fails.
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

	// Configure the GORM logger
	configureLogger()

	log.Info().Msg("Database initialized successfully")
	return nil
}

// createDBDirectory checks if the database path exists and creates it if it doesn't.
// It returns an error if the directory creation fails.
func createDBDirectory() error {
	if _, err := os.Stat(filepath.Dir(Path)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(Path), 0o750); err != nil {
			log.Error().Err(err).Msg("Failed to create database directory")
			return err
		}
	}
	return nil
}

// openDatabase opens the database connection.
// It returns an error if the database connection fails to open.
func openDatabase() error {
	var err error
	Db, err = gorm.Open(sqlite.Open(Path), &gorm.Config{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to initialize database")
		return err
	}
	return nil
}

// migrateTables creates the tables if they don't exist.
// It returns an error if the table migration fails.
func migrateTables() error {
	if err := Db.AutoMigrate(&Game{}); err != nil {
		log.Error().Err(err).Msg("Failed to auto-migrate database")
		return err
	}

	if err := Db.AutoMigrate(&Token{}); err != nil {
		log.Error().Err(err).Msg("Failed to auto-migrate database")
		return err
	}
	return nil
}

// configureLogger configures the GORM logger based on the environment variable.
// It sets the logger to silent mode if the DEBUG_GOGG environment variable is not set, otherwise it sets it to debug mode.
func configureLogger() {
	if zerolog.GlobalLevel() == zerolog.Disabled {
		Db.Logger = Db.Logger.LogMode(0) // Silent mode
	} else {
		Db.Logger = Db.Logger.LogMode(4) // Debug mode
	}
}

// CloseDB closes the database connection.
// It returns an error if the database connection fails to close.
func CloseDB() error {
	sqlDB, err := Db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get raw database connection")
		return err
	}
	return sqlDB.Close()
}
