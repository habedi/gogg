package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func downloadCmd() *cobra.Command {
	var language, platformName string
	var extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag bool
	var numThreads int

	cmd := &cobra.Command{
		Use:   "download [gameID] [downloadDir]",
		Short: "Download game files from GOG",
		Long:  "Download game files from GOG for the specified game ID to the specified directory",
		Args:  cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			gameID, err := strconv.Atoi(args[0])
			if err != nil {
				cmd.PrintErrln("Error: Invalid game ID. It must be a positive integer.")
				return
			}
			downloadDir := args[1]
			executeDownload(gameID, downloadDir, strings.ToLower(language), platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads)
		},
	}

	cmd.Flags().StringVarP(&language, "lang", "l", "en", "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", "windows", "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", true, "Include extra content files? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", true, "Include DLC files? [true, false]")
	cmd.Flags().BoolVarP(&resumeFlag, "resume", "r", true, "Resume downloading? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 5, "Number of worker threads to use for downloading [1-20]")
	cmd.Flags().BoolVarP(&flattenFlag, "flatten", "f", true, "Flatten the directory structure when downloading? [true, false]")
	cmd.Flags().BoolVarP(&skipPatchesFlag, "skip-patches", "s", false, "Skip patches when downloading? [true, false]")

	return cmd
}

func executeDownload(gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag bool, numThreads int) {
	log.Info().Msgf("Downloading games to %s...", downloadPath)
	log.Info().Msgf("Language: %s, Platform: %s, Extras: %v, DLC: %v", language, platformName, extrasFlag, dlcFlag)

	if numThreads < 1 || numThreads > 20 {
		fmt.Println("Number of threads must be between 1 and 20.")
		return
	}
	if !isValidLanguage(language) {
		fmt.Println("Invalid language code. Supported languages are:")
		for langCode, langName := range gameLanguages {
			fmt.Printf("'%s' for %s\n", langCode, langName)
		}
		return
	} else {
		language = gameLanguages[language]
	}

	user, err := auth.RefreshToken()
	if err != nil {
		fmt.Println("Failed to find or refresh the access token. Did you login?")
		return
	}

	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		log.Info().Msgf("Creating download path %s", downloadPath)
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
			return
		}
	}

	game, err := db.GetGameByID(gameID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game by ID.")
		fmt.Println("Error retrieving game from local catalogue.")
		return
	}
	if game == nil {
		log.Error().Msg("Game not found in the catalogue.")
		fmt.Printf("Game with ID %d not found in the local catalogue.\n", gameID)
		return
	}
	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse game details.")
		fmt.Println("Error parsing game data from local catalogue.")
		return
	}

	logDownloadParameters(parsedGameData, gameID, downloadPath, language, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads)

	ctx := context.Background()
	progressWriter := io.Writer(os.Stderr)

	err = client.DownloadGameFiles(ctx, user.AccessToken, parsedGameData, downloadPath, language, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads, progressWriter)
	if err != nil {
		if err == context.Canceled || err == context.DeadlineExceeded {
			log.Warn().Err(err).Msg("Download operation cancelled or timed out.")
			fmt.Println("\nDownload cancelled or timed out.")
		} else {
			log.Error().Err(err).Msg("Failed to download game files.")
			fmt.Printf("\nError downloading game files: %v\n", err)
		}
		return
	}

	fmt.Printf("\rGame files downloaded successfully to: \"%s\" \n", filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title)))
}

func logDownloadParameters(game client.Game, gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag bool, numThreads int) {
	fmt.Println("================================= Download Parameters =====================================")
	fmt.Printf("Downloading \"%v\" (with game ID=\"%d\") to \"%v\"\n", game.Title, gameID, downloadPath)
	fmt.Printf("Platform: \"%v\", Language: '%v'\n", platformName, language)
	fmt.Printf("Include Extras: %v, Include DLCs: %v, Resume enabled: %v\n", extrasFlag, dlcFlag, resumeFlag)
	fmt.Printf("Number of worker threads for download: %d\n", numThreads)
	fmt.Printf("Flatten directory structure: %v\n", flattenFlag)
	fmt.Printf("Skip patches: %v\n", skipPatchesFlag)
	fmt.Println("============================================================================================")
}
