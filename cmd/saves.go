package cmd

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/clierr"
	"github.com/habedi/gogg/pkg/config"
	"github.com/habedi/gogg/pkg/validation"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// savesCmd backs up a game's GOG Galaxy cloud saves to a local directory.
// It only ever reads from the cloud; nothing gogg does here can change or
// delete what GOG stores.
func savesCmd(authService *auth.Service) *cobra.Command {
	cfg := config.Load()

	var platformName string

	cmd := &cobra.Command{
		Use:   "saves [gameID] [outputDir]",
		Short: "Back up a game's cloud saves from GOG",
		Long: "Download the GOG Galaxy cloud saves of the specified game into the specified directory.\n" +
			"outputDir may be omitted when download_dir is set in ~/.config/gogg/config.json;\n" +
			"the saves then land under <download_dir>/saves/<game>.",
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			gameID, err := strconv.Atoi(args[0])
			if err != nil {
				cmd.PrintErrln("Error: Invalid game ID. It must be a positive integer.")
				return
			}
			if err := validation.ValidateGameID(gameID); err != nil {
				cmd.PrintErrln("Error:", err)
				return
			}

			gameRepo := db.NewGameRepository(db.GetDB())
			game, err := gameRepo.GetByID(cmd.Context(), gameID)
			if err != nil {
				fmt.Println(clierr.New(clierr.Internal, "Error retrieving game from local catalogue", err).Message)
				return
			}
			if game == nil {
				fmt.Println(clierr.New(clierr.NotFound, fmt.Sprintf("Game %d not found in local catalogue", gameID), nil).Message)
				return
			}

			var outputDir string
			if len(args) == 2 {
				outputDir = args[1]
			} else {
				if cfg.DownloadDir == "" {
					cmd.PrintErrln("Error: outputDir argument is required (or set download_dir in ~/.config/gogg/config.json)")
					return
				}
				outputDir = filepath.Join(cfg.DownloadDir, "saves", client.SanitizePath(game.Title))
			}

			ctx := cmd.Context()
			token, err := authService.RefreshTokenCtx(ctx)
			if err != nil {
				fmt.Println("Failed to find or refresh the access token. Did you login?")
				return
			}

			saves := client.NewCloudSaveClient()
			result, err := saves.BackupCloudSaves(ctx, token.RefreshToken, gameID, platformName, outputDir)
			if err != nil {
				if errors.Is(err, client.ErrCloudSavesUnavailable) {
					fmt.Printf("No cloud saves found for %s.\n", game.Title)
					return
				}
				log.Error().Err(err).Msg("Cloud save backup failed.")
				fmt.Println(clierr.New(clierr.Internal, "Failed to back up cloud saves", err).Message)
				return
			}

			for _, name := range result.Files {
				fmt.Println(name)
			}
			fmt.Printf("Backed up %d cloud save %s for %s to %s\n",
				len(result.Files), filesWord(len(result.Files)), game.Title, outputDir)
			if len(result.Skipped) > 0 {
				fmt.Printf("Skipped %d empty or unusable %s.\n", len(result.Skipped), filesWord(len(result.Skipped)))
			}
		},
	}

	cmd.Flags().StringVarP(&platformName, "platform", "p", cfg.Platform, "Platform whose build carries the game's cloud credentials [all, windows, mac, linux]")

	return cmd
}
