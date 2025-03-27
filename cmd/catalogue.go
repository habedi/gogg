package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// catalogueCmd represents the base command when called without any subcommands
// It returns a pointer to the created cobra.Command.
func catalogueCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalogue",
		Short: "Manage the game catalogue",
		Long:  "Manage the game catalogue by listing and searching for games, etc.",
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
// It returns a pointer to the created cobra.Command.
func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the list of games in the catalogue",
		Long:  "Show the list of all games in the catalogue in a tabular format",
		Run:   listGames,
	}
}

// listGames fetches and displays the list of games in the catalogue
// It takes a cobra.Command and a slice of strings as arguments.
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
		cmd.Println("Game catalogue is empty. Did you refresh the catalogue?")
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
// It returns a pointer to the created cobra.Command.
func infoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [gameID]",
		Short: "Show the information about a game in the catalogue",
		Long:  "Given a game ID, show detailed information about the game with the specified ID in JSON format",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			gameID, err := strconv.Atoi(args[0])
			if err != nil {
				cmd.PrintErrln("Error: Invalid game ID. It must be a number.")
				return
			}
			showGameInfo(cmd, gameID)
		},
	}
	return cmd
}

// showGameInfo fetches and displays detailed information about a game with the specified ID
// It takes a cobra.Command and an integer representing the game ID as arguments.
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
		cmd.Println("No game found with the specified ID. Please check the game ID.")
		return
	}

	// Unmarshal the nested JSON data
	var nestedData map[string]interface{}
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal nested game data")
		cmd.PrintErrln("Error: Failed to parse nested game data.")
		return
	}

	// Pretty print the nested JSON data
	nestedDataPretty, err := json.MarshalIndent(nestedData, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal nested game data")
		cmd.PrintErrln("Error: Failed to format nested game data.")
		return
	}

	// Print the nested JSON data
	cmd.Println(string(nestedDataPretty))
}

// refreshCmd refreshes the game catalogue with the latest data from the user's account
// It returns a pointer to the created cobra.Command.
func refreshCmd() *cobra.Command {
	// Define the number of threads to use for fetching game data
	var numThreads int

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Update the catalogue with the latest data from GOG",
		Long:  "Update the game catalogue with the latest data for the games owned by the user on GOG",
		Run: func(cmd *cobra.Command, args []string) {
			refreshCatalogue(cmd, numThreads)
		},
	}

	// Define the flag for the command
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 10,
		"Number of worker threads to use for fetching game data [1-20]")
	return cmd
}

// refreshCatalogue updates the game catalogue with the latest data from GOG
// It takes a cobra.Command and an integer representing the number of threads as arguments.
func refreshCatalogue(cmd *cobra.Command, numThreads int) {
	log.Info().Msg("Refreshing the game catalogue...")

	// Check the number of threads are valid
	if numThreads < 1 || numThreads > 20 {
		cmd.PrintErrln("Error: Number of threads should be between 1 and 20.")
		return
	}

	// Try to refresh the access token
	token, err := client.RefreshToken()
	if err != nil || token == nil {
		cmd.PrintErrln("Error: Failed to refresh the access token. Did you login?")
		return
	}

	games, err := client.FetchIdOfOwnedGames(token.AccessToken, "https://embed.gog.com/user/data/games")
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			cmd.PrintErrln("Error: Failed to fetch the list of owned games. Please use `login` command to login.")
		}
		log.Info().Msgf("Failed to fetch owned games: %v\n", err)
		return
	} else if len(games) == 0 {
		log.Info().Msg("No games found in the GOG account.")
		return
	} else {
		log.Info().Msgf("Found %d games IDs in the GOG account.\n", len(games))
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
				task.details, task.rawDetails, task.err = client.FetchGameData(token.AccessToken, url)
				if task.err != nil {
					log.Info().Msgf("Failed to fetch game details for game ID %d: %v\n", task.gameID, task.err)
				}
				// Process the task here instead of sending it back to the channel
				if task.err == nil && task.details.Title != "" {
					err = db.PutInGame(task.gameID, task.details.Title, task.rawDetails)
					if err != nil {
						log.Info().Msgf("Failed to insert game details for game ID %d: %v in the catalogue\n",
							task.gameID, err)
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

	// Wait for all workers to finish
	wg.Wait()

	// Finish the progress bar and check for any errors
	if err := bar.Finish(); err != nil {
		log.Error().Err(err).Msg("Failed to finish progress bar")
	}

	cmd.Println("Refreshed the game catalogue successfully.")
}

// searchCmd searches for games in the catalogue by ID or title.
// It returns a pointer to the created cobra.Command.
func searchCmd() *cobra.Command {
	var searchByIDFlag bool

	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for games in the catalogue",
		Long:  "Search for games in the catalogue given a query string, which can be a term in the title or a game ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			query := args[0]
			searchGames(cmd, query, searchByIDFlag)
		},
	}

	// Flag to determine the type of search
	cmd.Flags().BoolVarP(&searchByIDFlag, "id", "i", false,
		"Search by game ID instead of title?")

	return cmd
}

// searchGames searches for games in the catalogue by ID or title and displays the results in a table.
// It takes a cobra.Command, a string representing the query, and a boolean indicating whether to search by ID as arguments.
func searchGames(cmd *cobra.Command, query string, searchByID bool) {
	var games []db.Game
	var err error

	if searchByID {
		gameID, err := strconv.Atoi(query)
		if err != nil {
			cmd.PrintErrln("Error: Invalid game ID. It must be a number.")
			return
		}
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
	} else {
		log.Info().Msgf("Searching for games with term=%s in their title", query)
		games, err = db.SearchGamesByName(query)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to search games with term=%s in their title", query)
			cmd.PrintErrln("Error:", err)
			return
		}
	}

	if len(games) == 0 {
		cmd.Println("No game(s) found matching the query. Please check the search term or ID.")
		return
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Row ID", "Game ID", "Title"})
	table.SetColMinWidth(2, 50)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetRowLine(false)

	for i, game := range games {
		table.Append([]string{
			fmt.Sprintf("%d", i+1),
			fmt.Sprintf("%d", game.ID),
			game.Title,
		})
	}

	table.Render()
}

// exportCmd creates a new cobra.Command for exporting the game catalogue to a file.
// It returns a pointer to the created cobra.Command.
func exportCmd() *cobra.Command {
	var exportFormat string

	cmd := &cobra.Command{
		Use:   "export [exportDir]",
		Short: "Export the game catalogue to a file",
		Long:  "Export the game catalogue to a file in the specified path in the specified format",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			exportPath := args[0]
			exportCatalogue(cmd, exportPath, exportFormat)
		},
	}

	// Add flag for export format
	cmd.Flags().StringVarP(&exportFormat, "format", "f", "csv",
		"Format of the exported file [csv, json]")

	return cmd
}

// exportCatalogue handles the export logic for the game catalogue.
// It takes a cobra.Command, a string representing the export path, and a string representing the export format as arguments.
func exportCatalogue(cmd *cobra.Command, exportPath, exportFormat string) {
	log.Info().Msg("Exporting the game catalogue...")

	// Validate the export format
	switch exportFormat {
	case "json", "csv":
		// Valid formats
	default:
		log.Error().Msg("Invalid export format. Supported formats: json, csv")
		cmd.PrintErrln("Error: Invalid export format. Supported formats: json, csv")
		return
	}

	// Generate a timestamped filename
	timestamp := time.Now().Format("20060102_150405")
	var fileName string
	switch exportFormat {
	case "json":
		fileName = fmt.Sprintf("gogg_full_catalogue_%s.json", timestamp)
	case "csv":
		fileName = fmt.Sprintf("gogg_catalogue_%s.csv", timestamp)
	}

	filePath := filepath.Join(exportPath, fileName)

	// Export the catalogue based on the format
	var err error
	switch exportFormat {
	case "json":
		err = exportCatalogueToJSON(filePath)
	case "csv":
		err = exportCatalogueToCSV(filePath)
	}

	// Check for any errors during export
	if err != nil {
		log.Error().Err(err).Msg("Failed to export the game catalogue.")
		cmd.PrintErrln("Error: Failed to export the game catalogue.")
		return
	}

	cmd.Printf("Game catalogue exported successfully to: \"%s\"\n", filePath)
}

// exportCatalogueToCSV exports the game catalogue to a CSV file.
// It takes a string representing the file path as an argument and returns an error if any.
func exportCatalogueToCSV(path string) error {
	// Fetch all games from the catalogue
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	} else if len(games) == 0 {
		fmt.Println("No games found to export. Did you refresh the catalogue?")
	}

	// Make sure the directory of the path exists
	if err := ensurePathExists(path); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create CSV file %s", path)
		return err
	}
	defer file.Close()

	// Write the header
	if _, err := fmt.Fprintln(file, "ID,Title"); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header to file")
		return err
	}

	// Write the games
	for _, game := range games {
		if _, err := fmt.Fprintf(file, "%d,\"%s\"\n", game.ID, game.Title); err != nil {
			log.Error().Err(err).Msgf("Failed to write game %d to CSV file", game.ID)
			return err
		}
	}

	log.Info().Msgf("Game catalogue exported to CSV file: %s", path)
	return nil
}

// exportCatalogueToJSON exports the game catalogue to a JSON file.
// It takes a string representing the file path as an argument and returns an error if any.
func exportCatalogueToJSON(path string) error {
	// Fetch all games from the catalogue
	games, err := db.GetCatalogue()
	if err != nil {
		return err
	} else if len(games) == 0 {
		fmt.Println("No games found to export. Did you refresh the catalogue?")
		return nil
	}

	//  Make sure the directory of the path exists
	if err := ensurePathExists(path); err != nil {
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

// ensurePathExists checks if the directory of the given path exists and creates it if it doesn't.
// It takes a string representing the path and returns an error if any.
func ensurePathExists(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			log.Error().Err(err).Msgf("Failed to create directory %s", filepath.Dir(path))
			return err
		}
	}
	return nil
}
