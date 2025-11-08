package cmd

import (
	"context"
	"os"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/clierr"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var exitCodeByType = map[clierr.Type]int{
	clierr.Validation: 2,
	clierr.NotFound:   3,
	clierr.Download:   4,
	clierr.Internal:   1,
}

func Execute() {
	initializeDatabase()
	defer closeDatabase()

	gameRepo := db.NewGameRepository(db.GetDB())
	tokenRepo := db.NewTokenRepository(db.GetDB())
	gogClient := &client.GogClient{TokenURL: "https://auth.gog.com/token"}
	authService := auth.NewServiceWithRepo(tokenRepo, gogClient)

	rootCmd := createRootCmd(authService, gogClient, gameRepo)
	rootCmd.PersistentFlags().DurationP("timeout", "T", 0, "Global timeout for command execution (like 30s or 2m). 0 means no timeout")
	var cancel context.CancelFunc
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		to, err := cmd.Flags().GetDuration("timeout")
		if err != nil {
			return err
		}
		ctx := context.Background()
		if to > 0 {
			ctx, cancel = context.WithTimeout(ctx, to)
		}
		cmd.SetContext(ctx)
		return nil
	}
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if cancel != nil {
			cancel()
		}
	}

	if err := rootCmd.Execute(); err != nil {
		log.Error().Err(err).Msg("Command execution failed.")
		os.Exit(1)
	}
	if e := getLastCliErr(); e != nil { // mapped exit code
		if code, ok := exitCodeByType[e.Type]; ok {
			os.Exit(code)
		}
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
