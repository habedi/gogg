package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/clierr"
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

// getFileStatusString builds a compact string of current file progresses.
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
	sb.WriteString(fmt.Sprintf("Downloading %d files: ", len(files)))
	for i, file := range files {
		shortName := file
		if len(shortName) > 25 {
			shortName = "..." + shortName[len(shortName)-22:]
		}
		progress := cw.fileProgress[file]
		sizeStr := fmt.Sprintf("%s/%s", formatBytes(progress.current), formatBytes(progress.total))
		sb.WriteString(fmt.Sprintf("%s %s", shortName, sizeStr))
		if i < len(files)-1 {
			sb.WriteString(" | ")
		}
	}
	return sb.String()
}

func downloadCmd(authService *auth.Service) *cobra.Command {
	var language, platformName string
	var extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag bool
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
			if err := validation.ValidateGameID(gameID); err != nil {
				cmd.PrintErrln("Error:", err)
				return
			}
			downloadDir := args[1]
			ctx := cmd.Context()
			executeDownload(ctx, authService, gameID, downloadDir, language, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag, numThreads)
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
	cmd.Flags().BoolVar(&keepLatestFlag, "keep-latest", false, "Remove older installer versions after successful download (keep only highest version)")
	cmd.Flags().BoolVar(&rommLayoutFlag, "romm", false, "Use RomM compatible folder layout (platform/game)")

	return cmd
}

func executeDownload(ctx context.Context, authService *auth.Service, gameID int, downloadPath, language, platformName string, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, keepLatestFlag, rommLayoutFlag bool, numThreads int) {
	log.Info().Msgf("Downloading games to %s...", downloadPath)
	log.Info().Msgf("Language: %s, Platform: %s, Extras: %v, DLC: %v", language, platformName, extrasFlag, dlcFlag)

	if err := validation.ValidateThreadCount(numThreads); err != nil {
		fmt.Println(clierr.New(clierr.Validation, "Invalid thread count", err).Message)
		return
	}
	if err := validation.ValidatePlatform(platformName); err != nil {
		fmt.Println(clierr.New(clierr.Validation, "Invalid platform", err).Message)
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
		fmt.Println(clierr.New(clierr.Validation, "Invalid language code", nil).Message)
		for langCode, langName := range client.GameLanguages {
			fmt.Printf("'%s' for %s\n", langCode, langName)
		}
		return
	}

	user, err := authService.RefreshTokenCtx(ctx)
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

	gameRepo := db.NewGameRepository(db.GetDB())
	game, err := gameRepo.GetByID(ctx, gameID)
	if err != nil {
		fmt.Println(clierr.New(clierr.Internal, "Error retrieving game from local catalogue", err).Message)
		return
	}
	if game == nil {
		fmt.Println(clierr.New(clierr.NotFound, fmt.Sprintf("Game %d not found in local catalogue", gameID), nil).Message)
		return
	}
	parsedGameData, err := client.ParseGameData(game.Data)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse game details.")
		fmt.Println("Error parsing game data from local catalogue.")
		return
	}

	logDownloadParameters(parsedGameData, gameID, downloadPath, languageFullName, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, numThreads)

	progressWriter := &cliProgressWriter{}

	err = client.DownloadGameFiles(ctx, user.AccessToken, parsedGameData, downloadPath, languageFullName, platformName, extrasFlag, dlcFlag, resumeFlag, flattenFlag, skipPatchesFlag, rommLayoutFlag, numThreads, progressWriter)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fmt.Println(clierr.New(clierr.Internal, "Download cancelled or timed out", err).Message)
		} else {
			fmt.Println(clierr.New(clierr.Download, "Failed to download game files", err).Message)
		}
		return
	}

	fmt.Printf("\rGame files downloaded successfully to: \"%s\" \n", filepath.Join(downloadPath, client.SanitizePath(parsedGameData.Title)))
	if keepLatestFlag {
		if err := pruneOldVersions(downloadPath, parsedGameData.Title); err != nil {
			log.Warn().Err(err).Msg("Failed to prune old versions")
		}
	}
}

var versionPattern = regexp.MustCompile(`^(?P<prefix>.*?)(?P<ver>\d+(?:\.\d+)+)(?P<suffix>\.[^.]+)$`)

func parseVersion(filename string) (prefix string, verSlice []int, suffix string, ok bool) {
	m := versionPattern.FindStringSubmatch(filename)
	if m == nil {
		return "", nil, "", false
	}
	prefix = m[1]
	suffix = m[3]
	verParts := strings.Split(m[2], ".")
	for _, p := range verParts {
		v, err := strconv.Atoi(p)
		if err != nil {
			return "", nil, "", false
		}
		verSlice = append(verSlice, v)
	}
	return prefix, verSlice, suffix, true
}

func compareVersions(a, b []int) int { // 1 if a>b, -1 if a<b, 0 if eq
	for i := 0; i < len(a) || i < len(b); i++ {
		va, vb := 0, 0
		if i < len(a) {
			va = a[i]
		}
		if i < len(b) {
			vb = b[i]
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

func pruneOldVersions(downloadPath, title string) error {
	root := filepath.Join(downloadPath, client.SanitizePath(title))
	if _, err := os.Stat(root); err != nil {
		return err
	}
	// Candidate extensions (installer types)
	extAllowed := map[string]struct{}{".exe": {}, ".bin": {}, ".dmg": {}, ".sh": {}, ".zip": {}, ".tar.gz": {}, ".rar": {}}
	latestByPrefix := make(map[string]struct {
		file string
		ver  []int
	})
	filesByPrefix := make(map[string][]string)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		ext := filepath.Ext(name)
		// handle .tar.gz
		if strings.HasSuffix(name, ".tar.gz") {
			ext = ".tar.gz"
		}
		if _, ok := extAllowed[ext]; !ok {
			return nil
		}
		prefix, ver, _, ok := parseVersion(name)
		if !ok || len(ver) == 0 {
			return nil
		}
		filesByPrefix[prefix] = append(filesByPrefix[prefix], path)
		curr, exists := latestByPrefix[prefix]
		if !exists || compareVersions(ver, curr.ver) == 1 {
			latestByPrefix[prefix] = struct {
				file string
				ver  []int
			}{file: path, ver: ver}
		}
		return nil
	})
	// Remove older ones
	for prefix, files := range filesByPrefix {
		latest := latestByPrefix[prefix].file
		for _, f := range files {
			if f != latest {
				if err := os.Remove(f); err != nil {
					log.Warn().Err(err).Str("file", f).Msg("Failed to remove old version file")
				}
			}
		}
	}
	return nil
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
