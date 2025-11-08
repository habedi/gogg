package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/validation"
	"github.com/olekukonko/tablewriter"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

func catalogueCmd(authService *auth.Service, gameRepo db.GameRepository) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalogue",
		Short: "Manage the game catalogue",
		Long:  "Manage the game catalogue by listing and searching for games, etc.",
	}
	cmd.AddCommand(
		listCmd(gameRepo),
		searchCmd(gameRepo),
		infoCmd(gameRepo),
		refreshCmd(authService),
		exportCmd(gameRepo),
	)
	return cmd
}

func listCmd(repo db.GameRepository) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show the list of games in the catalogue",
		Long:  "Show the list of all games in the catalogue in a tabular format",
		Run:   func(cmd *cobra.Command, args []string) { listGames(cmd, repo) },
	}
}

func listGames(cmd *cobra.Command, repo db.GameRepository) {
	log.Info().Msg("Listing all games in the catalogue...")
	games, err := repo.List(cmd.Context())
	if err != nil {
		cmd.PrintErrln("Error: Unable to list games. Please check the logs for details.")
		log.Error().Err(err).Msg("Failed to fetch games from the game catalogue.")
		return
	}
	if len(games) == 0 {
		cmd.Println("Game catalogue is empty. Did you refresh the catalogue?")
		return
	}
	table := tablewriter.NewWriter(cmd.OutOrStdout())
	table.SetHeader([]string{"Row ID", "Game ID", "Game Title"})
	table.SetColMinWidth(2, 60)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for i, game := range games {
		cleanedTitle := strings.ReplaceAll(game.Title, "\n", " ")
		table.Append([]string{
			fmt.Sprintf("%d", i+1),
			fmt.Sprintf("%d", game.ID),
			cleanedTitle,
		})
	}
	table.Render()
	log.Info().Msgf("Successfully listed %d games in the catalogue.", len(games))
}

func infoCmd(repo db.GameRepository) *cobra.Command {
	var updatesOnly bool
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
			if err := validation.ValidateGameID(gameID); err != nil {
				cmd.PrintErrln("Error:", err)
				return
			}
			showGameInfo(cmd, repo, gameID, updatesOnly)
		},
	}
	cmd.Flags().BoolVar(&updatesOnly, "updates", false, "Show a concise list of downloadable files and their versions")
	return cmd
}

func showGameInfo(cmd *cobra.Command, repo db.GameRepository, gameID int, updatesOnly bool) {
	if gameID == 0 {
		cmd.PrintErrln("Error: ID of the game is required to fetch information.")
		return
	}
	log.Info().Msgf("Fetching info for game with ID=%d", gameID)
	game, err := repo.GetByID(cmd.Context(), gameID)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to fetch info for game with ID=%d", gameID)
		cmd.PrintErrln("Error:", err)
		return
	}
	if game == nil {
		log.Info().Msgf("No game found with ID=%d", gameID)
		cmd.Println("No game found with the specified ID. Please check the game ID.")
		return
	}

	if !updatesOnly {
		var nestedData map[string]interface{}
		if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
			log.Error().Err(err).Msg("Failed to unmarshal nested game data")
			cmd.PrintErrln("Error: Failed to parse nested game data.")
			return
		}
		nestedDataPretty, err := json.MarshalIndent(nestedData, "", "  ")
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal nested game data")
			cmd.PrintErrln("Error: Failed to format nested game data.")
			return
		}
		cmd.Println(string(nestedDataPretty))
		return
	}

	// If --updates flag is used, show the version table
	var gameData client.Game
	if err := json.Unmarshal([]byte(game.Data), &gameData); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal game data for update view")
		cmd.PrintErrln("Error: Failed to parse game data.")
		return
	}

	table := tablewriter.NewWriter(cmd.OutOrStdout())
	table.SetHeader([]string{"Component", "Language", "Platform", "File Name", "Version", "Date"})
	table.SetAutoWrapText(false)
	table.SetRowLine(true)

	addFilesToTable := func(component string, downloads []client.Downloadable) {
		for _, dl := range downloads {
			platforms := map[string][]client.PlatformFile{
				"Windows": dl.Platforms.Windows,
				"Mac":     dl.Platforms.Mac,
				"Linux":   dl.Platforms.Linux,
			}
			for pName, pFiles := range platforms {
				if len(pFiles) == 0 {
					continue
				}
				for _, file := range pFiles {
					version := "N/A"
					if file.Version != nil {
						version = *file.Version
					}
					date := "N/A"
					if file.Date != nil {
						date = *file.Date
					}
					table.Append([]string{component, dl.Language, pName, file.Name, version, date})
				}
			}
		}
	}

	addFilesToTable(gameData.Title, gameData.Downloads)
	for _, dlc := range gameData.DLCs {
		addFilesToTable(fmt.Sprintf("DLC: %s", dlc.Title), dlc.ParsedDownloads)
	}

	table.Render()
}

func refreshCmd(authService *auth.Service) *cobra.Command {
	var numThreads int
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Update the catalogue with the latest data from GOG",
		Long:  "Update the game catalogue with the latest data for the games owned by the user on GOG",
		Run: func(cmd *cobra.Command, args []string) {
			refreshCatalogue(cmd, authService, numThreads)
		},
	}
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 10,
		"Number of worker threads to use for fetching game data [1-20]")
	return cmd
}

func refreshCatalogue(cmd *cobra.Command, authService *auth.Service, numThreads int) {
	log.Info().Msg("Refreshing the game catalogue...")
	if err := validation.ValidateThreadCount(numThreads); err != nil {
		cmd.PrintErrln("Error:", err)
		return
	}

	bar := progressbar.NewOptions(1000,
		progressbar.OptionSetDescription("Refreshing catalogue..."),
		progressbar.OptionSetWidth(20),
		progressbar.OptionShowIts(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionSetWriter(cmd.ErrOrStderr()), // Use command's error stream
	)

	progressCb := func(progress float64) {
		_ = bar.Set(int(progress * 1000))
	}

	err := client.RefreshCatalogue(cmd.Context(), authService, numThreads, progressCb)
	if err != nil {
		cmd.PrintErrln("Error: Failed to refresh catalogue. Please check the logs for details.")
		log.Error().Err(err).Msg("Failed to refresh the game catalogue")
		return
	}

	cmd.Println("Refreshed the game catalogue successfully.")
}

func searchCmd(repo db.GameRepository) *cobra.Command {
	var searchByIDFlag bool
	cmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search for games in the catalogue",
		Long:  "Search for games in the catalogue given a query string, which can be a term in the title or a game ID",
		Args:  cobra.ExactArgs(1),
		Run:   func(cmd *cobra.Command, args []string) { searchGames(cmd, repo, args[0], searchByIDFlag) },
	}
	cmd.Flags().BoolVarP(&searchByIDFlag, "id", "i", false,
		"Search by game ID instead of title?")
	return cmd
}

func searchGames(cmd *cobra.Command, repo db.GameRepository, query string, searchByID bool) {
	var games []db.Game
	var err error
	ctx := cmd.Context()
	if searchByID {
		gameID, err := strconv.Atoi(query)
		if err != nil {
			cmd.PrintErrln("Error: Invalid game ID. It must be a number.")
			return
		}
		log.Info().Msgf("Searching for game with ID=%d", gameID)
		game, err := repo.GetByID(ctx, gameID)
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
		games, err = repo.SearchByTitle(ctx, query)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to search games with term=%s in their title",
				query)
			cmd.PrintErrln("Error:", err)
			return
		}
	}
	if len(games) == 0 {
		cmd.Println("No game(s) found matching the query. Please check the search term or ID.")
		return
	}
	table := tablewriter.NewWriter(cmd.OutOrStdout())
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

func exportCmd(repo db.GameRepository) *cobra.Command {
	var exportFormat string
	cmd := &cobra.Command{
		Use:   "export [exportDir]",
		Short: "Export the game catalogue to a file",
		Long:  "Export the game catalogue to a file in the specified path in the specified format",
		Args:  cobra.ExactArgs(1),
		Run:   func(cmd *cobra.Command, args []string) { exportCatalogue(cmd, repo, args[0], exportFormat) },
	}
	cmd.Flags().StringVarP(&exportFormat, "format", "f", "csv",
		"Format of the exported file [csv, json]")
	return cmd
}

func exportCatalogue(cmd *cobra.Command, repo db.GameRepository, exportPath, exportFormat string) {
	log.Info().Msg("Exporting the game catalogue...")
	ctx := cmd.Context()
	games, err := repo.List(ctx)
	if err != nil {
		cmd.PrintErrln("Error: Failed to list games from the repository.")
		log.Error().Err(err).Msg("Failed to list games from the repository")
		return
	} else if len(games) == 0 {
		cmd.Println("No games found to export. Did you refresh the catalogue?")
		return
	}
	switch exportFormat {
	case "json", "csv":
	default:
		log.Error().Msg("Invalid export format. Supported formats: json, csv")
		cmd.PrintErrln("Error: Invalid export format. Supported formats: json, csv")
		return
	}
	timestamp := time.Now().Format("20060102_150405")
	var fileName string
	switch exportFormat {
	case "json":
		fileName = fmt.Sprintf("gogg_full_catalogue_%s.json", timestamp)
	case "csv":
		fileName = fmt.Sprintf("gogg_catalogue_%s.csv", timestamp)
	}
	filePath := filepath.Join(exportPath, fileName)
	var writeErr error
	switch exportFormat {
	case "json":
		writeErr = exportCatalogueToJSON(filePath, games)
	case "csv":
		writeErr = exportCatalogueToCSV(filePath, games)
	}
	if writeErr != nil {
		log.Error().Err(writeErr).Msg("Failed to export the game catalogue.")
		cmd.PrintErrln("Error: Failed to export the game catalogue.")
		return
	}
	cmd.Printf("Game catalogue exported successfully to: \"%s\"\n", filePath)
}

func exportCatalogueToCSV(path string, games []db.Game) error {
	if err := ensurePathExists(path); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create CSV file %s", path)
		return err
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			log.Error().Err(cerr).Msgf("Failed to close CSV file %s", path)
		}
	}()
	if _, err := fmt.Fprintln(file, "ID,Title"); err != nil {
		log.Error().Err(err).Msg("Failed to write CSV header to file")
		return err
	}
	for _, game := range games {
		if _, err := fmt.Fprintf(file, "%d,\"%s\"\n", game.ID, game.Title); err != nil {
			log.Error().Err(err).Msgf("Failed to write game %d to CSV file", game.ID)
			return err
		}
	}
	log.Info().Msgf("Game catalogue exported to CSV file: %s", path)
	return nil
}

func exportCatalogueToJSON(path string, games []db.Game) error {
	if err := ensurePathExists(path); err != nil {
		return err
	}
	writeGamesToJSON := func(path string, games []db.Game) error {
		file, err := os.Create(path)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create JSON file %s", path)
			return err
		}
		defer func() {
			if cerr := file.Close(); cerr != nil {
				log.Error().Err(cerr).Msgf("Failed to close JSON file %s", path)
			}
		}()
		if err := json.NewEncoder(file).Encode(games); err != nil {
			log.Error().Err(err).Msg("Failed to write games to JSON file")
			return err
		}
		log.Info().Msgf("Game catalogue exported to JSON file: %s", path)
		return nil
	}
	return writeGamesToJSON(path, games)
}

func ensurePathExists(path string) error {
	if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			log.Error().Err(err).Msgf("Failed to create directory %s", filepath.Dir(path))
			return err
		}
	}
	return nil
}
