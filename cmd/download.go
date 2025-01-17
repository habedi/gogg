package cmd

import (
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"os"
)

// Map of supported game languages and their native names
var gameLanguages = map[string]string{
	"en":      "English",
	"fr":      "Français",            // French
	"de":      "Deutsch",             // German
	"es":      "Español",             // Spanish
	"it":      "Italiano",            // Italian
	"ru":      "Русский",             // Russian
	"pl":      "Polski",              // Polish
	"pt-BR":   "Português do Brasil", // Portuguese (Brazil)
	"zh-Hans": "简体中文",                // Simplified Chinese
	"ja":      "日本語",                 // Japanese
	"ko":      "한국어",                 // Korean
}

// downloadCmd downloads a selected game from GOG
func downloadCmd() *cobra.Command {
	var gameID int
	var downloadDir string
	var language string
	var platformName string
	var extrasFlag bool
	var dlcFlag bool
	var resumeFlag bool
	var numThreads int

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download game files from GOG",
		Run: func(cmd *cobra.Command, args []string) {
			executeDownload(gameID, downloadDir, language, platformName, extrasFlag, dlcFlag, resumeFlag, numThreads)
		},
	}

	// Add flags for download options
	cmd.Flags().IntVarP(&gameID, "id", "i", 0, "ID of the game to download (required)")
	cmd.Flags().StringVarP(&downloadDir, "dir", "o", "", "Directory to store the downloaded files (required)")
	cmd.Flags().StringVarP(&language, "lang", "l", "en", "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", "windows", "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", true, "Include extras? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", true, "Include DLC? [true, false]")
	cmd.Flags().BoolVarP(&resumeFlag, "resume", "r", true, "Resume downloading? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 5, "Number of threads to use for downloading (default: 5)")

	// Game ID flag and download path are required
	if err := cmd.MarkFlagRequired("id"); err != nil {
		log.Error().Err(err).Msg("Failed to mark 'id' flag as required.")
	}

	if err := cmd.MarkFlagRequired("path"); err != nil {
		log.Error().Err(err).Msg("Failed to mark 'path' flag as required.")
	}

	return cmd
}

// executeDownload handles the download logic
func executeDownload(gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag bool, numThreads int) {
	log.Info().Msgf("Downloading games to %s...\n", downloadPath)
	log.Info().Msgf("Language: %s, Platform: %s, Extras: %v, DLC: %v\n", language, platformName, extrasFlag, dlcFlag)

	// Check the number of threads is valid
	if numThreads < 1 || numThreads > 20 {
		fmt.Println("Number of threads must be between 1 and 20.")
		return
	}

	// Try to refresh the access token
	if _, err := authenticateUser(false); err != nil {
		log.Error().Msg("Failed to refresh the access token.")
		return
	}

	// Check if the download path exists, if not, create it
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		log.Info().Msgf("Creating download path %s\n", downloadPath)
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			log.Error().Err(err).Msgf("Failed to create download path %s\n", downloadPath)
			return
		}
	}

	// Fetch the game details from the catalogue
	game, err := db.GetGameByID(gameID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get game by ID.")
		return
	} else if game == nil {
		log.Error().Msg("Game not found in the catalogue.")
		return
	}

	// Parse the game data into a Game object
	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse game details.")
		return
	}

	// Load the user data
	user, err := db.GetUserData()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve user data from the database.")
		return
	}

	// Show download parameters to the user
	logDownloadParameters(parsedGameData, gameID, downloadPath, language, platformName, extrasFlag, dlcFlag, resumeFlag,
		numThreads)

	// Download the game files
	err = client.DownloadGameFiles(user.AccessToken, parsedGameData, downloadPath, gameLanguages[language],
		platformName, extrasFlag, dlcFlag, resumeFlag, numThreads)
	if err != nil {
		log.Error().Err(err).Msg("Failed to download game files.")
	}
	fmt.Println("\rGame files downloaded successfully.")
}

// logDownloadParameters logs the download parameters
func logDownloadParameters(game client.Game, gameID int, downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag bool, numThreads int) {
	fmt.Println("================================= Download Parameters =====================================")
	fmt.Printf("Downloading \"%v\" (with game ID=\"%d\") to \"%v\"\n", game.Title, gameID, downloadPath)
	fmt.Printf("Platform: \"%v\", Language: '%v'\n", platformName, gameLanguages[language])
	fmt.Printf("Include Extras: \"%v, Include DLCs: \"%v\", Resume enabled: \"%v\"\n", extrasFlag, dlcFlag, resumeFlag)
	fmt.Printf("Number of worker threads for download: \"%d\"\n", numThreads)
	fmt.Println("============================================================================================")
}
