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
	Db   *gorm.DB
	Path string
)

func init() {
	ConfigurePath()
}

// ConfigurePath determines and sets the database path based on environment variables.
// It is public to allow for re-evaluation during testing.
func ConfigurePath() {
	var baseDir string

	// 1. Check for explicit GOGG_HOME override
	if goggHome := os.Getenv("GOGG_HOME"); goggHome != "" {
		baseDir = goggHome
	} else if xdgDataHome := os.Getenv("XDG_DATA_HOME"); xdgDataHome != "" {
		// 2. Check for XDG_DATA_HOME convention (e.g., ~/.local/share)
		baseDir = filepath.Join(xdgDataHome, "gogg")
	} else {
		// 3. Fallback to the default in the user's home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal().Err(err).Msg("Could not determine user home directory")
		}
		baseDir = filepath.Join(homeDir, ".gogg")
	}

	Path = filepath.Join(baseDir, "games.db")
}

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

	configureLogger()
	log.Info().Str("path", Path).Msg("Database initialized successfully")
	return nil
}

// createDBDirectory checks if the database path exists and creates it if it doesn't.
// It returns an error if the directory creation fails.
func createDBDirectory() error {
	dbDir := filepath.Dir(Path)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dbDir, 0o750); err != nil {
			log.Error().Err(err).Msgf("Failed to create database directory: %s", dbDir)
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
	if Db == nil {
		return nil // Nothing to close
	}
	sqlDB, err := Db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get raw database connection")
		return err
	}
	return sqlDB.Close()
}
