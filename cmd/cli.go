package cmd

import (
	"context"
	"os"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func Execute() {
	initializeDatabase()
	defer closeDatabase()

	gameRepo := db.NewGameRepository(db.GetDB())
	tokenRepo := db.NewTokenRepository(db.GetDB())
	gogClient := &client.GogClient{TokenURL: "https://auth.gog.com/token"}
	authService := auth.NewServiceWithRepo(tokenRepo, gogClient)

	rootCmd := createRootCmd(authService, gogClient, gameRepo)
	rootCmd.PersistentFlags().DurationP("timeout", "T", 0, "Global timeout for command execution (e.g. 30s, 2m). 0 means no timeout")
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		to, err := cmd.Flags().GetDuration("timeout")
		if err != nil {
			return err
		}
		ctx := context.Background()
		if to > 0 {
			ctx, _ = context.WithTimeout(ctx, to)
		}
		cmd.SetContext(ctx)
		return nil
	}
	rootCmd.PersistentFlags().BoolP("help", "h", false, "Show help for a command")

	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command execution failed.")
		os.Exit(1)
	}
}

func createRootCmd(authService *auth.Service, gogClient *client.GogClient, gameRepo db.GameRepository) *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "gogg",
		Short: "A Downloader for GOG",
	}

	rootCmd.AddCommand(
		catalogueCmd(authService, gameRepo),
		downloadCmd(authService),
		versionCmd(),
		loginCmd(gogClient),
		fileCmd(),
		guiCmd(authService),
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
