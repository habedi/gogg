package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/clierr"
	"github.com/habedi/gogg/pkg/config"
	"github.com/habedi/gogg/pkg/validation"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
)

// formatBytes converts a byte count into a human-readable string (KB, MB, GB).
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// cliProgressWriter handles progress updates for the CLI.
type cliProgressWriter struct {
	bar             *progressbar.ProgressBar
	fileProgress    map[string]struct{ current, total int64 }
	fileBytes       map[string]int64
	downloadedBytes int64
	mu              sync.RWMutex
}

func (cw *cliProgressWriter) Write(p []byte) (n int, err error) {
	scanner := bufio.NewScanner(strings.NewReader(string(p)))
	for scanner.Scan() {
		var update client.ProgressUpdate
		if err := json.Unmarshal(scanner.Bytes(), &update); err == nil {
			cw.mu.Lock()
			switch update.Type {
			case "start":
				cw.bar = progressbar.NewOptions64(
					update.OverallTotalBytes,
					progressbar.OptionSetDescription("Downloading..."),
					progressbar.OptionSetWriter(os.Stderr),
					progressbar.OptionShowBytes(true),
					progressbar.OptionThrottle(200*time.Millisecond),
					progressbar.OptionClearOnFinish(),
					progressbar.OptionSpinnerType(14),
				)
				cw.fileProgress = make(map[string]struct{ current, total int64 })
				cw.fileBytes = make(map[string]int64)
				cw.downloadedBytes = 0
			case "file_progress":
				if cw.bar != nil {
					diff := update.CurrentBytes - cw.fileBytes[update.FileName]
					cw.fileBytes[update.FileName] = update.CurrentBytes
					cw.downloadedBytes += diff
					_ = cw.bar.Set64(cw.downloadedBytes)

					cw.fileProgress[update.FileName] = struct{ current, total int64 }{update.CurrentBytes, update.TotalBytes}
					if update.CurrentBytes >= update.TotalBytes && update.TotalBytes > 0 {
						delete(cw.fileProgress, update.FileName)
					}
					cw.bar.Describe(cw.getFileStatusString())
				}
			}
			cw.mu.Unlock()
		}
	}
	return len(p), nil
}

// statusFilesShown is how many in-flight files the bar's description names. A
// description longer than the terminal is wide wraps, and every redraw of a
// wrapped bar becomes a new line of scrollback.
const statusFilesShown = 2

// getFileStatusString builds a compact string of current file progresses,
// short enough to keep the bar on its one line.
func (cw *cliProgressWriter) getFileStatusString() string {
	if len(cw.fileProgress) == 0 {
		return "Finalizing..."
	}

	files := make([]string, 0, len(cw.fileProgress))
	for f := range cw.fileProgress {
		files = append(files, f)
	}
	sort.Strings(files)

	var sb strings.Builder
	fmt.Fprintf(&sb, "%d %s: ", len(files), filesWord(len(files)))
	for i, file := range files {
		if i >= statusFilesShown {
			fmt.Fprintf(&sb, " +%d more", len(files)-statusFilesShown)
			break
		}
		shortName := file
		if len(shortName) > 20 {
			shortName = "..." + shortName[len(shortName)-17:]
		}
		progress := cw.fileProgress[file]
		percent := 0
		if progress.total > 0 {
			percent = int(float64(progress.current) / float64(progress.total) * 100)
		}
		if i > 0 {
			sb.WriteString(" | ")
		}
		fmt.Fprintf(&sb, "%s %d%%", shortName, percent)
	}
	return sb.String()
}

func filesWord(n int) string {
	if n == 1 {
		return "file"
	}
	return "files"
}

func downloadCmd(authService *auth.Service) *cobra.Command {
	cfg := config.Load()

	var language, platformName string
	var extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag, lutrisLayoutFlag, noVerifyFlag bool
	var numThreads int
	var connections int

	cmd := &cobra.Command{
		Use:   "download [gameID] [downloadDir]",
		Short: "Download game files from GOG",
		Long: "Download game files from GOG for the specified game ID to the specified directory.\n" +
			"downloadDir may be omitted when download_dir is set in ~/.config/gogg/config.json.",
		Args: cobra.RangeArgs(1, 2),
		Run: func(cmd *cobra.Command, args []string) {
			gameID, err := strconv.Atoi(args[0])
			if err != nil {
				e := clierr.New(clierr.Validation, "Invalid game ID. It must be a positive integer.", err)
				cmd.PrintErrln("Error: Invalid game ID. It must be a positive integer.")
				setLastCliErr(e)
				return
			}
			if err := validation.ValidateGameID(gameID); err != nil {
				e := clierr.New(clierr.Validation, err.Error(), err)
				cmd.PrintErrln("Error:", err)
				setLastCliErr(e)
				return
			}
			var downloadDir string
			if len(args) == 2 {
				downloadDir = args[1]
			} else {
				downloadDir = cfg.DownloadDir
				if downloadDir == "" {
					e := clierr.New(clierr.Validation, "downloadDir argument is required (or set download_dir in ~/.config/gogg/config.json)", nil)
					cmd.PrintErrln("Error: downloadDir argument is required (or set download_dir in ~/.config/gogg/config.json)")
					setLastCliErr(e)
					return
				}
			}
			ctx := cmd.Context()
			executeDownload(ctx, authService, gameID, downloadDir, language, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag, lutrisLayoutFlag, noVerifyFlag, numThreads, connections)
		},
	}

	cmd.Flags().StringVarP(&language, "lang", "l", cfg.Language, "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", cfg.Platform, "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", cfg.Extras, "Include extra content files? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", cfg.DLCs, "Include DLC files? [true, false]")
	cmd.Flags().BoolVarP(&resumeFlag, "resume", "r", cfg.Resume, "Resume downloading? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", cfg.Threads, "Number of worker threads to use for downloading [1-20]")
	cmd.Flags().IntVar(&connections, "connections", cfg.Connections, "Number of connections per file for large files [1-8]; more than one splits a file into ranges downloaded at once")
	cmd.Flags().BoolVarP(&flattenFlag, "flatten", "f", cfg.Flatten, "Flatten the directory structure when downloading? [true, false]")
	cmd.Flags().BoolVarP(&skipPatchesFlag, "skip-patches", "s", cfg.SkipPatches, "Skip patches when downloading? [true, false]")
	cmd.Flags().BoolVar(&keepLatestFlag, "keep-latest", cfg.KeepLatest, "Remove older installer versions after successful download (keep only highest version)")
	cmd.Flags().BoolVar(&rommLayoutFlag, "romm", cfg.RommLayout, "Use RomM compatible folder layout (platform/game)")
	cmd.Flags().BoolVar(&lutrisLayoutFlag, "lutris", cfg.LutrisLayout, "Use Lutris compatible folder layout (game-slug/gog), so Lutris reuses the files as its installer cache")
	cmd.Flags().BoolVar(&noVerifyFlag, "no-verify", cfg.NoVerify, "Do not check downloaded files against the MD5 GOG publishes; checksums are still recorded in files.json")

	return cmd
}

func executeDownload(ctx context.Context, authService *auth.Service, gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag, lutrisLayoutFlag, noVerifyFlag bool, numThreads, connections int) {
	log.Info().Msgf("Downloading games to %s...", downloadPath)
	log.Info().Msgf("Language: %s, Platform: %s, Extras: %v, DLC: %v", language, platformName, extrasFlag, dlcFlag)

	if err := validation.ValidateThreadCount(numThreads); err != nil {
		e := clierr.New(clierr.Validation, "Invalid thread count", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}
	// Configs written before the key existed decode to zero, which means
	// the setting was never chosen; that is the single stream, not an error.
	if connections == 0 {
		connections = 1
	}
	if err := validation.ValidateConnectionCount(connections); err != nil {
		e := clierr.New(clierr.Validation, "Invalid connection count", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}
	if err := validation.ValidatePlatform(platformName); err != nil {
		e := clierr.New(clierr.Validation, "Invalid platform", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}

	var languageFullName string
	found := false
	for code, full := range client.GameLanguages {
		if strings.EqualFold(code, language) {
			languageFullName = full
			found = true
			break
		}
	}
	if !found {
		e := clierr.New(clierr.Validation, "Invalid language code", nil)
		fmt.Println(e.Message)
		setLastCliErr(e)
		for langCode, langName := range client.GameLanguages {
			fmt.Printf("'%s' for %s\n", langCode, langName)
		}
		return
	}

	user, err := authService.RefreshTokenCtx(ctx)
	if err != nil {
		e := clierr.New(clierr.Internal, "Failed to find or refresh the access token. Did you login?", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}

	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		log.Info().Msgf("Creating download path %s", downloadPath)
		if err := os.MkdirAll(downloadPath, os.ModePerm); err != nil {
			log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
			e := clierr.New(clierr.Internal, fmt.Sprintf("Failed to create download path %s", downloadPath), err)
			setLastCliErr(e)
			return
		}
	}

	gameRepo := db.NewGameRepository(db.GetDB())
	game, err := gameRepo.GetByID(ctx, gameID)
	if err != nil {
		e := clierr.New(clierr.Internal, "Error retrieving game from local catalogue", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}
	if game == nil {
		e := clierr.New(clierr.NotFound, fmt.Sprintf("Game %d not found in local catalogue", gameID), nil)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}
	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse game details.")
		e := clierr.New(clierr.Internal, "Error parsing game data from local catalogue.", err)
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}
	parsedGameData.ID = game.ID

	logDownloadParameters(parsedGameData, gameID, downloadPath, languageFullName, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads)
	if noVerifyFlag {
		fmt.Println("Checksum verification is off. Each checksum is still recorded in files.json, but it is not checked against GOG.")
	}

	progressWriter := &cliProgressWriter{}

	err = client.DownloadGameFiles(ctx, user.AccessToken, parsedGameData, downloadPath,
		client.DownloadOptions{
			Language: languageFullName, Platform: platformName,
			Extras: extrasFlag, DLCs: dlcFlag, Resume: resumeFlag,
			Flatten: flattenFlag, SkipPatches: skipPatchesFlag, RomMLayout: rommLayoutFlag,
			LutrisLayout: lutrisLayoutFlag, SkipVerify: noVerifyFlag,
			Threads: numThreads, Connections: connections,
		}, progressWriter)
	if err != nil {
		var e *clierr.Error
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			e = clierr.New(clierr.Internal, "Download cancelled or timed out", err)
		} else {
			e = clierr.New(clierr.Download, "Failed to download game files", err)
		}
		fmt.Println(e.Message)
		setLastCliErr(e)
		return
	}

	var gameDir string
	switch {
	case lutrisLayoutFlag:
		gameDir = filepath.Join(downloadPath, client.LutrisSlug(parsedGameData.Title), "gog")
	case rommLayoutFlag:
		plat := client.RomMPlatform(platformName)
		if plat == "all" {
			gameDir = downloadPath
		} else {
			gameDir = filepath.Join(downloadPath, plat, client.SanitizePath(parsedGameData.Title))
		}
	default:
		gameDir = filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title))
	}
	fmt.Printf("\rGame files downloaded successfully to: \"%s\" \n", gameDir)
	if keepLatestFlag {
		removed, pruneErr := client.PruneOldInstallerVersions(downloadPath, parsedGameData.Title,
			client.DownloadOptions{RomMLayout: rommLayoutFlag, LutrisLayout: lutrisLayoutFlag, Platform: platformName})
		if pruneErr != nil {
			log.Warn().Err(pruneErr).Msg("Failed to prune old versions")
		}
		if len(removed) > 0 {
			fmt.Printf("Removed %d older installer %s.\n", len(removed), filesWord(len(removed)))
		}
	}
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
