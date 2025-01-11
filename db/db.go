package db

import (
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"os"
	"path/filepath"
)

// Database variables
var (
	Db   *gorm.DB                                             // GORM database instance
	Path = filepath.Join(os.Getenv("HOME"), ".gogg/games.db") // Default database path
)

// InitDB initializes the database and creates the tables if they don't exist.
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

// createDBDirectory checks if the database path exists and creates it if it doesn't.
func createDBDirectory() error {
	if _, err := os.Stat(filepath.Dir(Path)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(Path), 0755); err != nil {
			log.Error().Err(err).Msg("Failed to create database directory")
			return err
		}
	}
	return nil
}

// openDatabase opens the database connection.
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
func migrateTables() error {
	if err := Db.AutoMigrate(&Game{}); err != nil {
		log.Error().Err(err).Msg("Failed to auto-migrate database")
		return err
	}

	if err := Db.AutoMigrate(&User{}); err != nil {
		log.Error().Err(err).Msg("Failed to auto-migrate database")
		return err
	}
	return nil
}

// configureLogger configures the GORM logger based on environment variable.
func configureLogger() {
	if os.Getenv("DEBUG_GOGG") == "" {
		Db.Logger = Db.Logger.LogMode(0) // Silent mode
	} else {
		Db.Logger = Db.Logger.LogMode(4) // Debug mode
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
