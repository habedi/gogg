package cmd

import (
	"os"

	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func Execute() {
	rootCmd := createRootCmd()
	initializeDatabase()
	defer closeDatabase()

	rootCmd.PersistentFlags().BoolP("help", "h", false, "Show help for a command")

	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command execution failed.")
		os.Exit(1)
	}
}

func createRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gogg",
		Short: "A Downloader for GOG",
	}

	rootCmd.AddCommand(
		catalogueCmd(),
		downloadCmd(),
		versionCmd(),
		loginCmd(),
		fileCmd(),
		guiCmd(),
	)

	rootCmd.CompletionOptions.HiddenDefaultCmd = true
	rootCmd.SetHelpCommand(&cobra.Command{
		Use:    "no-help",
		Hidden: true,
	})

	return rootCmd
}

func initializeDatabase() {
	if err := db.InitDB(); err != nil {
		log.Error().Err(err).Msg("Failed to initialize database")
		os.Exit(1)
	}
}

func closeDatabase() {
	if err := db.CloseDB(); err != nil {
		log.Error().Err(err).Msg("Failed to close the database.")
		os.Exit(1)
	}
}
