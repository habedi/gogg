package db

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	// Db is the global database connection object
	Db *gorm.DB
	// Path is the default path to the SQLite database file
	Path = getDefaultDBPath()
)

// getDefaultDBPath returns the default path to the SQLite database file
// in a cross-platform way.
func getDefaultDBPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fallback to current directory if home directory can't be determined
		return filepath.Join(".", ".gogg/games.db")
	}
	return filepath.Join(homeDir, ".gogg/games.db")
}

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
