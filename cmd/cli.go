package cmd

import (
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"os"
)

// Execute runs the root command of Gogg
func Execute() {
	rootCmd := createRootCmd()
	initializeDatabase()
	defer closeDatabase()

	// Add a global flags to the root command
	rootCmd.PersistentFlags().BoolP("help", "h", false, "help for `gogg` and its commands")

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command execution failed")
		os.Exit(1) // Exit with a failure code
	}
}

// createRootCmd defines the root command and adds subcommands
func createRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gogg",
		Short: "A downloader for GOG",
		Long:  "Gogg is a minimalistic command-line tool to download games from GOG.com",
	}

	// Add subcommands to the root command
	rootCmd.AddCommand(
		initCmd(),
		authCmd(),
		catalogueCmd(),
		downloadCmd(),
		versionCmd(),
	)

	// Hide the completion command and replace the help command
	rootCmd.CompletionOptions.HiddenDefaultCmd = true // Hides 'completion'
	rootCmd.SetHelpCommand(&cobra.Command{            // Replace 'help' with a hidden command
		Use:    "no-help", // This helped turn off the help command :)
		Hidden: true,
	})

	return rootCmd
}

// initializeDatabase initializes Gogg's internal database
func initializeDatabase() {
	if err := db.InitDB(); err != nil {
		log.Info().Msgf("Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
}

// closeDatabase closes the database connection
func closeDatabase() {
	if err := db.CloseDB(); err != nil {
		log.Error().Err(err).Msg("Failed to close the database")
		os.Exit(1)
	}
}
