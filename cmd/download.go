// cmd/download.go
package cmd

import (
	"context" // Import context
	"fmt"
	"io" // Import io
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

// downloadCmd function remains the same...
func downloadCmd() *cobra.Command {
	var language string
	var platformName string
	var extrasFlag bool
	var dlcFlag bool
	var resumeFlag bool
	var numThreads int
	var flattenFlag bool

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
			// Call the updated executeDownload
			executeDownload(gameID, downloadDir, strings.ToLower(language), platformName, extrasFlag, dlcFlag,
				resumeFlag, flattenFlag, numThreads)
		},
	}

	// Flags remain the same...
	cmd.Flags().StringVarP(&language, "lang", "l", "en", "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", "windows", "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", true, "Include extra content files? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", true, "Include DLC files? [true, false]")
	cmd.Flags().BoolVarP(&resumeFlag, "resume", "r", true, "Resume downloading? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 5, "Number of worker threads to use for downloading [1-20]")
	cmd.Flags().BoolVarP(&flattenFlag, "flatten", "f", true, "Flatten the directory structure when downloading? [true, false]") // Default changed to true

	return cmd
}

// executeDownload handles the download logic for a specified game.
func executeDownload(gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag bool,
	flattenFlag bool, numThreads int,
) {
	log.Info().Msgf("Downloading games to %s...", downloadPath)
	log.Info().Msgf("Language: %s, Platform: %s, Extras: %v, DLC: %v", language, platformName, extrasFlag, dlcFlag)

	// --- Validation (same as before) ---
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
		language = gameLanguages[language] // Use full name
	}

	// --- Token Refresh (same as before) ---
	if _, err := client.RefreshToken(); err != nil {
		fmt.Println("Failed to find or refresh the access token. Did you login?")
		return
	}

	// --- Path Creation (same as before) ---
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		log.Info().Msgf("Creating download path %s", downloadPath)
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
			return
		}
	}

	// --- Fetch Game Data (same as before) ---
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

	// --- Get Token (same as before) ---
	user, err := db.GetTokenRecord()
	if err != nil {
		log.Error().Err(err).Msg("Failed to retrieve user token from the database.")
		fmt.Println("Error retrieving user token. Please try logging in again.")
		return
	}
	if user == nil {
		log.Error().Msg("User token record is nil.")
		fmt.Println("User token not found. Please try logging in again.")
		return
	}

	// --- Log Parameters (same as before) ---
	logDownloadParameters(parsedGameData, gameID, downloadPath, language, platformName, extrasFlag, dlcFlag, resumeFlag,
		flattenFlag, numThreads)

	// --- Create Context and Progress Writer ---
	// For CLI, use background context (no explicit cancel button here)
	// We could potentially wire this up to OS signals (like SIGINT) later if needed.
	ctx := context.Background()
	// For CLI, the progress bar should write to standard error (stderr) by default
	// Pass os.Stderr explicitly to match the updated function signature
	progressWriter := io.Writer(os.Stderr)

	// --- Call Updated Download Function ---
	// Pass ctx and progressWriter
	err = client.DownloadGameFiles(
		ctx, // Pass context
		user.AccessToken,
		parsedGameData,
		downloadPath,
		language,
		platformName,
		extrasFlag,
		dlcFlag,
		resumeFlag,
		flattenFlag,
		numThreads,
		progressWriter, // Pass progress writer (stderr)
	)
	// --- Handle Result (same as before, but progress appears on stderr) ---
	if err != nil {
		// Check for context cancellation (though less likely in basic CLI)
		if err == context.Canceled || err == context.DeadlineExceeded {
			log.Warn().Err(err).Msg("Download operation cancelled or timed out.")
			fmt.Println("\nDownload cancelled or timed out.") // Add newline after progress bar potentially
		} else {
			log.Error().Err(err).Msg("Failed to download game files.")
			fmt.Printf("\nError downloading game files: %v\n", err) // Add newline
		}
		return // Return on error
	}

	// Use \r to clear potential progress bar line before final message, then \n
	fmt.Printf("\rGame files downloaded successfully to: \"%s\" \n", filepath.Join(downloadPath,
		client.SanitizePath(parsedGameData.Title)))
}

// logDownloadParameters function remains the same...
func logDownloadParameters(game client.Game, gameID int, downloadPath, language, platformName string,
	extrasFlag, dlcFlag, resumeFlag bool, flattenFlag bool, numThreads int,
) {
	fmt.Println("================================= Download Parameters =====================================")
	fmt.Printf("Downloading \"%v\" (with game ID=\"%d\") to \"%v\"\n", game.Title, gameID, downloadPath)
	fmt.Printf("Platform: \"%v\", Language: '%v'\n", platformName, language)
	fmt.Printf("Include Extras: %v, Include DLCs: %v, Resume enabled: %v\n", extrasFlag, dlcFlag, resumeFlag) // Cleaner bool formatting
	fmt.Printf("Number of worker threads for download: %d\n", numThreads)
	fmt.Printf("Flatten directory structure: %v\n", flattenFlag)
	fmt.Println("============================================================================================")
}

// isValidLanguage function remains the same...
// (needs access to gameLanguages map from cmd/shared.go)
