package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// catalogueCmd represents the base command when called without any subcommands
func catalogueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalogue",
		Short: "Manage the game catalogue",
	}

	// Add subcommands to the catalogue command
	cmd.AddCommand(
		listCmd(),
		searchCmd(),
		infoCmd(),
		refreshCmd(),
		exportCmd(),
	)

	return cmd
}

// listCmd shows the list of games in the catalogue
func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the list all games in the catalogue",
		Run:   listGames,
	}
}

func listGames(cmd *cobra.Command, args []string) {
	log.Info().Msg("Listing all games in the catalogue...")

	// Fetch the list of games in the catalogue
	games, err := db.GetCatalogue()
	if err != nil {
		cmd.PrintErrln("Error: Unable to list games. Please check the logs for details.")
		log.Error().Err(err).Msg("Failed to fetch games from the game catalogue.")
		return
	}

	// Check if there are any games to display
	if len(games) == 0 {
		cmd.Println("No games found in the catalogue. Use `gogg catalogue refresh` to update the catalogue.")
		return
	}

	// Create a table for displaying the games
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Row ID", "Game ID", "Game Title"})

	// Table appearance settings
	table.SetColMinWidth(2, 60)                      // Set minimum width for the Title column
	table.SetAlignment(tablewriter.ALIGN_LEFT)       // Align all columns to the left
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT) // Align headers to the left
	table.SetAutoWrapText(false)                     // Disable text wrapping in all columns
	table.SetRowLine(false)                          // Disable row line breaks

	// Populate the table with game data
	for i, game := range games {
		// Clean the title to remove line breaks or unnecessary spaces
		cleanedTitle := strings.ReplaceAll(game.Title, "\n", " ")
		table.Append([]string{
			fmt.Sprintf("%d", i+1),     // Row ID
			fmt.Sprintf("%d", game.ID), // Game ID
			cleanedTitle,               // Title
		})
	}

	// Render the table
	table.Render()

	log.Info().Msgf("Successfully listed %d games in the catalogue.", len(games))
}

// infoCmd shows detailed information about a specific game, given its ID or title
func infoCmd() *cobra.Command {
	var gameID int
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show information about a specific game",
		Run: func(cmd *cobra.Command, args []string) {
			showGameInfo(cmd, gameID)
		},
	}

	// Define the flag for the command
	cmd.Flags().IntVarP(&gameID, "id", "i", 0, "ID of the game to show its information")

	// Mark the flag as required and handle any errors
	if err := cmd.MarkFlagRequired("id"); err != nil {
		log.Error().Err(err).Msg("Failed to mark 'id' flag as required")
	}

	return cmd
}

func showGameInfo(cmd *cobra.Command, gameID int) {
	if gameID == 0 {
		cmd.PrintErrln("Error: ID of the game is required to fetch information.")
		return
	}

	log.Info().Msgf("Fetching info for game with ID=%d", gameID)

	// Retrieve the game from the catalogue
	game, err := db.GetGameByID(gameID)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to fetch info for game with ID=%d", gameID)
		cmd.PrintErrln("Error:", err)
		return
	}

	// Check if the game was found
	if game == nil {
		log.Info().Msgf("No game found with ID=%d", gameID)
		cmd.Println("No game found with the specified ID.")
		return
	}

	// Display the game information
	cmd.Println("Game Information:")
	cmd.Printf("ID: %d\n", game.ID)
	cmd.Printf("Title: %s\n", game.Title)
	cmd.Printf("Data: %s\n", game.Data)
}

// refreshCmd refreshes the game catalogue with the latest data from the user's account
func refreshCmd() *cobra.Command {

	// Define the number of threads to use for fetching game data
	var numThreads int

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Update the catalogue with the latest data from the GOG account",
		Run: func(cmd *cobra.Command, args []string) {
			refreshCatalogue(cmd, numThreads)
		},
	}

	// Define the flag for the command
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 5, "Number of threads to use for fetching game data from the GOG")
	return cmd
}

func refreshCatalogue(cmd *cobra.Command, numThreads int) {
	log.Info().Msg("Refreshing the game catalogue...")

	// Check the number of threads are valid
	if numThreads < 1 || numThreads > 20 {
		cmd.PrintErrln("Error: Number of threads should be between 1 and 20.")
		return
	}

	// Try to refresh the access token
	user, err := authenticateUser(false)
	if err != nil {
		cmd.PrintErrln("Error: Failed to authenticate. Please check your credentials and try again.")
	}

	if user == nil {
		cmd.PrintErrln("Error: No user data found. Please run 'gogg init' to enter your username and password.")
		return
	}

	games, err := client.FetchIdOfOwnedGames(user.AccessToken, "https://embed.gog.com/user/data/games")
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			cmd.PrintErrln("Error: Failed to fetch the list of owned games. Please use `auth` command to re-authenticate.")
		}
		log.Info().Msgf("Failed to fetch owned games: %v\n", err)
		return
	} else if len(games) == 0 {
		log.Info().Msg("No games found in your GOG account.")
		return
	} else {
		log.Info().Msgf("Found %d games IDs in your GOG account.\n", len(games))
	}

	if err := db.EmptyCatalogue(); err != nil {
		log.Fatal().Err(err).Msg("Failed to empty the game catalogue.")
		return
	}

	log.Info().Msg("Games table truncated. Starting data refresh...")

	bar := progressbar.NewOptions(len(games),
		progressbar.OptionSetDescription("Refreshing catalogue..."),
		progressbar.OptionSetWidth(20),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionClearOnFinish(),
	)

	// Define a task struct to hold the game details and any errors encountered during fetching
	type gameTask struct {
		gameID     int
		details    client.Game
		rawDetails string
		err        error
	}

	taskChan := make(chan gameTask, 10)
	var wg sync.WaitGroup

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", task.gameID)
				task.details, task.rawDetails, task.err = client.FetchGameData(user.AccessToken, url)
				if task.err != nil {
					log.Info().Msgf("Failed to fetch game details for game ID %d: %v\n", task.gameID, task.err)
				}
				// Process the task here instead of sending it back to the channel
				if task.err == nil && task.details.Title != "" {
					err = db.PutInGame(task.gameID, task.details.Title, task.rawDetails)
					if err != nil {
						log.Info().Msgf("Failed to insert game details for game ID %d: %v in the catalogue\n", task.gameID, err)
					}
				}
				_ = bar.Add(1)
			}
		}()
	}

	go func() {
		for _, gameID := range games {
			taskChan <- gameTask{gameID: gameID}
		}
		close(taskChan)
	}()

	wg.Wait()
	bar.Finish()
	cmd.Printf("Refreshing completed successfully. There are %d games in the catalogue.\n", len(games))
}

// searchCmd searches for games in the catalogue by ID or title
func searchCmd() *cobra.Command {
	var gameID int
	var searchTerm string
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search for games in the catalogue by ID or title",
		Run: func(cmd *cobra.Command, args []string) {
			searchGames(cmd, gameID, searchTerm)
		},
	}

	// Flags for search
	cmd.Flags().IntVarP(&gameID, "id", "i", 0, "ID of the game to search")
	cmd.Flags().StringVarP(&searchTerm, "term", "t", "", "Search term to search for;"+
		" search is case-insensitive and does partial matching of the term with the game title")
	return cmd
}

func searchGames(cmd *cobra.Command, gameID int, searchTerm string) {
	if gameID == 0 && searchTerm == "" {
		cmd.PrintErrln("Error: one of the flags --id or --term is required. Use `gogg catalogue search -h` for more information.")
		return
	}

	// Check not both flags are provided
	if gameID != 0 && searchTerm != "" {
		cmd.PrintErrln("Error: only one of the flags --id or --term is required. Use `gogg catalogue search -h` for more information.")
		return
	}

	var games []db.Game
	var err error

	// Search by game ID
	if gameID != 0 {
		log.Info().Msgf("Searching for game with ID=%d", gameID)
		game, err := db.GetGameByID(gameID)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to fetch game with ID=%d", gameID)
			cmd.PrintErrln("Error:", err)
			return
		}
		if game != nil {
			games = append(games, *game)
		}
	}

	// Search by search term
	if searchTerm != "" {
		log.Info().Msgf("Searching for games with term=%s in its title", searchTerm)
		games, err = db.SearchGamesByName(searchTerm)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to search games with term=%s in its title", searchTerm)
			cmd.PrintErrln("Error:", err)
			return
		}
	}

	// Check if any games were found
	if len(games) == 0 {
		cmd.Printf("No game(s) found matching the search criteria.\n")
		return
	}

	// Display the search results in a table format
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Row ID", "Game ID", "Title"})
	table.SetColMinWidth(2, 50)                      // Set minimum width for the Title column
	table.SetAlignment(tablewriter.ALIGN_LEFT)       // Align all columns to the left
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT) // Align headers to the left
	table.SetAutoWrapText(false)                     // Disable text wrapping in all columns
	table.SetRowLine(false)                          // Disable row line breaks

	for i, game := range games {
		table.Append([]string{
			fmt.Sprintf("%d", i+1),     // Row ID
			fmt.Sprintf("%d", game.ID), // Game ID
			game.Title,                 // Game Title
		})
	}

	table.Render()
}

// exportCmd exports the game catalogue to a file in JSON or CSV format based on the user's choice
func exportCmd() *cobra.Command {
	exportPath := ""
	exportFormat := ""

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the game catalogue to a file",
		Run: func(cmd *cobra.Command, args []string) {
			exportCatalogue(cmd, exportPath, exportFormat)
		},
	}

	// Add flags for export path and format
	cmd.Flags().StringVarP(&exportPath, "dir", "d", "", "Directory to export the file (required)")
	cmd.Flags().StringVarP(&exportFormat, "format", "f", "", "Export format: json or csv (required)")

	// Mark flags as required
	cmd.MarkFlagRequired("path")
	cmd.MarkFlagRequired("format")

	return cmd
}

func exportCatalogue(cmd *cobra.Command, exportPath, exportFormat string) {
	log.Info().Msg("Exporting the game catalogue...")

	// Validate the export path
	if exportPath == "" {
		log.Error().Msg("Export path is required.")
		cmd.PrintErrln("Error: Export path is required.")
		return
	}

	// Ensure the directory exists or create it
	if err := os.MkdirAll(exportPath, os.ModePerm); err != nil {
		log.Error().Err(err).Msg("Failed to create export directory.")
		cmd.PrintErrln("Error: Failed to create export directory.")
		return
	}

	// Validate the export format
	if exportFormat != "json" && exportFormat != "csv" {
		log.Error().Msg("Invalid export format. Supported formats: json, csv")
		cmd.PrintErrln("Error: Invalid export format. Supported formats: json, csv")
		return
	}

	// Generate a timestamped filename
	timestamp := time.Now().Format("20060102_150405")
	var fileName string
	if exportFormat == "json" {
		fileName = fmt.Sprintf("gogg_full_catalogue_%s.json", timestamp)
	} else if exportFormat == "csv" {
		fileName = fmt.Sprintf("gogg_catalogue_%s.csv", timestamp)
	}

	filePath := filepath.Join(exportPath, fileName)

	// Export the catalogue based on the format
	var err error
	if exportFormat == "json" {
		err = exportCatalogueToJSON(filePath)
	} else if exportFormat == "csv" {
		err = exportCatalogueToCSV(filePath)
	}

	// Check for any errors during export
	if err != nil {
		log.Error().Err(err).Msg("Failed to export the game catalogue.")
		cmd.PrintErrln("Error: Failed to export the game catalogue.")
		return
	}

	log.Info().Msgf("Game catalogue exported successfully to %s.", filePath)
}

// exportCatalogueToCSV exports the game catalogue to a CSV file.
func exportCatalogueToCSV(path string) error {

	// Fetch all games from the catalogue
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}

	// writeGamesToCSV writes the games to a CSV file.
	writeGamesToCSV := func(path string, games []db.Game) error {
		file, err := os.Create(path)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create CSV file %s", path)
			return err
		}
		defer file.Close()

		// Write the header
		if _, err := file.WriteString("ID,Title\n"); err != nil {
			log.Error().Err(err).Msg("Failed to write CSV header to file")
			return err
		}

		// Write the games
		for _, game := range games {
			if _, err := file.WriteString(fmt.Sprintf("%d,\"%s\"\n", game.ID, game.Title)); err != nil {
				log.Error().Err(err).Msgf("Failed to write game %d to CSV file", game.ID)
				return err
			}
		}

		log.Info().Msgf("Game catalogue exported to CSV file: %s", path)
		return nil
	}

	// Write the game catalogue to a CSV file
	return writeGamesToCSV(path, games)
}

// exportCatalogueToJSON exports the game catalogue to a JSON file.
func exportCatalogueToJSON(path string) error {

	// Fetch all games from the catalogue
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	}

	// writeGamesToJSON writes the games to a JSON file.
	writeGamesToJSON := func(path string, games []db.Game) error {
		file, err := os.Create(path)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create JSON file %s", path)
			return err
		}
		defer file.Close()

		// Write the games
		if err := json.NewEncoder(file).Encode(games); err != nil {
			log.Error().Err(err).Msg("Failed to write games to JSON file")
			return err
		}

		log.Info().Msgf("Game catalogue exported to JSON file: %s", path)
		return nil
	}

	// Write the full game catalogue to a JSON file
	return writeGamesToJSON(path, games)
}
