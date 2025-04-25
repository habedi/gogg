package cmd

import (
	"os"

	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// Execute runs the root command of Gogg.
// It initializes the database, sets up the root command, and executes it.
func Execute() {
	rootCmd := createRootCmd()
	initializeDatabase()
	defer closeDatabase()

	// Add a global flag to the root command
	rootCmd.PersistentFlags().BoolP("help", "h", false, "Show help for a command")

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command execution failed.")
		os.Exit(1) // Exit with a failure code
	}
}

// createRootCmd defines the root command and adds subcommands.
// It returns a pointer to the created cobra.Command.
func createRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gogg",
		Short: "A Downloader for GOG",
	}

	// Add subcommands to the root command
	rootCmd.AddCommand(
		catalogueCmd(),
		downloadCmd(),
		versionCmd(),
		loginCmd(),
		fileCmd(),
		guiCmd(),
	)

	// Hide the completion command and replace the help command
	rootCmd.CompletionOptions.HiddenDefaultCmd = true // Hides 'completion'
	rootCmd.SetHelpCommand(&cobra.Command{            // Replace 'help' with a hidden command
		Use:    "no-help", // This helped turn off the help command :)
		Hidden: true,
	})

	return rootCmd
}

// initializeDatabase initializes Gogg's internal database.
// It logs an error and exits the program if the database initialization fails.
func initializeDatabase() {
	if err := db.InitDB(); err != nil {
		log.Error().Err(err).Msg("Failed to initialize database")
		os.Exit(1)
	}
}

// closeDatabase closes the database connection.
// It logs an error and exits the program if closing the database fails.
func closeDatabase() {
	if err := db.CloseDB(); err != nil {
		log.Error().Err(err).Msg("Failed to close the database.")
		os.Exit(1)
	}
}
