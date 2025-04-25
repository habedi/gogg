// client/download.go
package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
)

// downloadTask represents a file download job.
type downloadTask struct {
	url      string
	fileName string // This should be the *intended* final filename
	subDir   string
	resume   bool
	flatten  bool
}

// SanitizePath sanitizes a string to be used as a file path by removing special characters,
// removing spaces, replacing colons with hyphens, and converting to lowercase.
func SanitizePath(name string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"®", ""},
		{" ", ""},
		{":", "-"},
		{"(", ""},
		{")", ""},
	}
	name = strings.ToLower(name)
	for _, r := range replacements {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	return name
}

// createDirIfNotExist checks for a directory and creates it if needed.
func createDirIfNotExist(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path %s exists but is not a directory", path)
		}
		return nil // Directory already exists
	}
	if os.IsNotExist(err) {
		log.Debug().Msgf("Creating directory: %s", path)
		return os.MkdirAll(path, os.ModePerm) // Use 0755 or more specific permissions if needed
	}
	// Another error occurred (e.g., permission error)
	return err
}

// findRedirectLocation makes a GET request without following redirects to check for a Location header.
func findRedirectLocation(accessToken, urlStr string, client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}
	// Authorization *is* needed for embed.gog.com URLs
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// GOG uses 302 Found for download redirects primarily
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			log.Debug().Str("from", urlStr).Str("to", location).Msg("Download URL redirected")
			// Ensure the redirect location is absolute
			redirectURL, parseErr := resp.Request.URL.Parse(location) // Use parseErr
			if parseErr != nil {
				log.Warn().Err(parseErr).Str("base", resp.Request.URL.String()).Str("location", location).Msg("Failed to parse relative redirect URL")
				return "", fmt.Errorf("failed to parse redirect URL: %w", parseErr)
			}
			return redirectURL.String(), nil // Return absolute URL
		}
		log.Warn().Str("url", urlStr).Msg("Redirect status received but Location header is missing")
	} else if resp.StatusCode != http.StatusOK {
		// Only log warning, main GET will confirm error
		log.Warn().Str("url", urlStr).Int("status", resp.StatusCode).Msg("Unexpected status code when checking for redirect")
	}
	return "", nil // No redirect found or needed
}

// processDownloadTask handles a single download task.
func processDownloadTask(task downloadTask, accessToken, downloadPath, gameTitle string, client, clientNoRedirect *http.Client) error {
	urlStr := task.url        // This is the embed.gog.com URL or the redirected URL
	fileName := task.fileName // Use the filename passed in the task
	subDir := task.subDir
	resume := task.resume
	flatten := task.flatten

	log.Debug().Str("initial_url", task.url).Str("target_filename", fileName).Msg("Processing download task")

	// Check for redirect location using the non-redirect client.
	if redirectURL, errRedirect := findRedirectLocation(accessToken, urlStr, clientNoRedirect); errRedirect == nil && redirectURL != "" { // Use different var name for redirect error
		log.Info().Str("from", urlStr).Str("to", redirectURL).Msg("Following download redirect")
		urlStr = redirectURL
		// NOTE: We *keep* the original task.fileName, as the redirected URL might have
		// an obscure CDN filename. The original ManualURL base name is usually desired.
	} else if errRedirect != nil {
		// Log only, proceed with original URL
		log.Warn().Err(errRedirect).Str("url", urlStr).Msg("Failed to check for redirect, proceeding with original URL")
	}

	// Filename is already set from the task (and was decoded before task creation).

	// Determine final subdirectory path
	if flatten {
		subDir = ""
	} else {
		subDir = SanitizePath(subDir)
	}

	gameDir := SanitizePath(gameTitle)
	finalDirPath := filepath.Join(downloadPath, gameDir, subDir)
	filePath := filepath.Join(finalDirPath, fileName) // Use the intended filename

	if err := createDirIfNotExist(finalDirPath); err != nil {
		log.Error().Err(err).Str("path", finalDirPath).Msg("Failed to create directory")
		return err // Return error creating directory
	}

	var file *os.File
	var startOffset int64 = 0
	var err error // Declare err here for the whole block

	// Handle file creation/resume logic
	if resume {
		var fileInfo os.FileInfo          // Declare fileInfo here
		fileInfo, err = os.Stat(filePath) // Assign to outer err
		if err == nil {
			startOffset = fileInfo.Size()
			log.Info().Str("file", fileName).Int64("offset", startOffset).Msg("Resuming download")
			// Assign result of OpenFile to outer err
			file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil { // Check the OpenFile error
				log.Error().Err(err).Str("path", filePath).Msg("Failed to open file for appending")
				return err // Return the OpenFile error
			}
		} else if os.IsNotExist(err) { // Check the Stat error
			log.Info().Str("file", fileName).Msg("Starting new download (resume enabled, file not found)")
			// Assign result of Create to outer err
			file, err = os.Create(filePath)
			if err != nil { // Check the Create error
				log.Error().Err(err).Str("path", filePath).Msg("Failed to create file")
				return err // Return the Create error
			}
		} else { // Handle other Stat errors
			log.Error().Err(err).Str("path", filePath).Msg("Failed to stat file for resume")
			return err // Return the Stat error
		}
	} else {
		log.Info().Str("file", fileName).Msg("Starting new download (resume disabled)")
		// Assign result of Create to outer err
		file, err = os.Create(filePath)
		if err != nil { // Check the Create error
			log.Error().Err(err).Str("path", filePath).Msg("Failed to create file")
			return err // Return the Create error
		}
	}
	// If file creation/opening succeeded, file is not nil here. Defer close.
	if file == nil {
		// This case should ideally not happen if error handling above is correct, but check defensively
		return fmt.Errorf("internal error: file handle is nil after open/create logic for %s", filePath)
	}
	defer file.Close()

	// Declare totalSize before the HEAD request block
	var totalSize int64 = -1 // Initialize to -1 (unknown)

	// Use HEAD request to get total file size
	headReq, headErr := http.NewRequest("HEAD", urlStr, nil) // Use different var name for head error
	if headErr != nil {
		log.Error().Err(headErr).Str("url", urlStr).Msg("Failed to create HEAD request")
		// Don't return yet, GET might still work
	} else {
		// Auth required for HEAD on embed.gog.com/redirect target
		headReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		headResp, headRespErr := client.Do(headReq) // Use different var name for head response error
		if headRespErr != nil {
			log.Warn().Err(headRespErr).Str("url", urlStr).Msg("HEAD request failed")
		} else {
			// Only try to read body if there might be one (e.g., non-200 status)
			var bodyBytes []byte
			if headResp.StatusCode != http.StatusOK {
				bodyBytes, _ = io.ReadAll(headResp.Body)
			}
			headResp.Body.Close() // Close body immediately after potential read or if status OK

			if headResp.StatusCode == http.StatusOK {
				// Assign to the outer totalSize
				totalSize = headResp.ContentLength
				log.Debug().Str("file", fileName).Int64("size", totalSize).Msg("Got file size via HEAD")
			} else {
				log.Warn().Str("url", urlStr).Int("status", headResp.StatusCode).Str("body", string(bodyBytes)).Msg("Unexpected status code from HEAD request")
			}
		}
	}

	// Check if file is already complete (use outer totalSize)
	// Ensure totalSize was successfully determined (is >= 0) before comparing with startOffset
	if resume && totalSize >= 0 && startOffset == totalSize {
		log.Info().Str("file", fileName).Int64("size", totalSize).Msg("File already fully downloaded. Skipping.")
		fmt.Printf("Skipping %s (already complete).\n", fileName)
		return nil
	}
	// Check for oversized local file (use outer totalSize)
	if resume && totalSize >= 0 && startOffset > totalSize {
		log.Warn().Str("file", fileName).Int64("offset", startOffset).Int64("totalSize", totalSize).Msg("Local file larger than remote size. Restarting download.")
		startOffset = 0
		if err = file.Truncate(0); err != nil { // Assign truncate error to outer err
			log.Error().Err(err).Str("path", filePath).Msg("Failed to truncate oversized file")
			return err // Return truncate error
		}
		if _, err = file.Seek(0, io.SeekStart); err != nil { // Assign seek error to outer err
			log.Error().Err(err).Str("path", filePath).Msg("Failed to seek to start after truncate")
			return err // Return seek error
		}
	}

	// Prepare the GET request
	getReq, err := http.NewRequest("GET", urlStr, nil) // Can redeclare err here or use outer one
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Failed to create GET request")
		return err
	}
	// Auth required for GET on embed.gog.com/redirect target
	getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if resume && startOffset > 0 {
		getReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		log.Debug().Str("file", fileName).Int64("offset", startOffset).Msg("Added Range header")
	}

	// Execute the GET request
	getResp, err := client.Do(getReq) // Can redeclare err here or use outer one
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("GET request failed")
		return err
	}
	defer getResp.Body.Close()

	// Check response status
	if getResp.StatusCode != http.StatusOK && getResp.StatusCode != http.StatusPartialContent {
		bodyBytes, _ := io.ReadAll(getResp.Body)
		err = fmt.Errorf("unexpected HTTP status: %s. Body: %s", getResp.Status, string(bodyBytes))
		log.Error().Err(err).Str("url", urlStr).Int("status", getResp.StatusCode).Msg("Download request failed")
		getResp.Body.Close() // Ensure body is closed after reading
		return err
	}

	// Determine size for progress bar (use outer totalSize if known)
	sizeForProgress := totalSize
	if sizeForProgress < 0 { // If HEAD failed or didn't run
		sizeForProgress = getResp.ContentLength
		if sizeForProgress > 0 {
			log.Debug().Str("file", fileName).Int64("size", sizeForProgress).Msg("Got file size via GET ContentLength")
			// Add the offset back to get the true total size for the progress bar
			if resume && startOffset > 0 {
				sizeForProgress += startOffset
			}
		} else {
			log.Warn().Str("file", fileName).Msg("Could not determine file size from GET ContentLength. Progress bar may be inaccurate.")
			// Keep sizeForProgress as -1 if ContentLength is also unknown
		}
	}

	// Setup progress bar
	barDescription := fmt.Sprintf("Downloading %s", fileName)
	maxDescLen := 50
	if len(barDescription) > maxDescLen {
		barDescription = barDescription[:maxDescLen-3] + "..."
	}

	progressBar := progressbar.NewOptions64(
		sizeForProgress, // Use the determined size, could be -1
		progressbar.OptionSetDescription(barDescription),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer: "#", SaucerPadding: " ", BarStart: "[", BarEnd: "]",
		}),
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSpinnerType(14),
		progressbar.OptionSetVisibility(sizeForProgress >= 0), // Show only if size is known
	)

	// Set initial progress for resuming downloads
	if err = progressBar.Set64(startOffset); err != nil { // Assign progress bar error to outer err
		log.Warn().Err(err).Msg("Failed to set progress bar offset")
		// Continue without setting offset
	}

	// Create progress reader and copy
	progressReader := progressbar.NewReader(getResp.Body, progressBar)
	written, err := io.Copy(file, &progressReader) // Assign io.Copy error to outer err
	if err != nil {
		log.Error().Err(err).Str("file", fileName).Msg("Error during download stream copy")
		return fmt.Errorf("failed to save file %s: %w", filePath, err) // Return the copy error
	}
	log.Info().Str("file", fileName).Int64("bytes", written).Msg("Download stream finished")

	// Final check (use sizeForProgress which includes the offset)
	// Check only if size was known (sizeForProgress >= 0)
	if sizeForProgress >= 0 && (startOffset+written) != sizeForProgress {
		log.Warn().Str("file", fileName).
			Int64("expected", sizeForProgress). // Expected total including offset
			Int64("actual", startOffset+written).
			Msg("Downloaded size does not match expected size")
	}

	fmt.Printf("Finished downloading %s\n", fileName)
	return nil // Success
}

// DownloadGameFiles downloads game files, extras, and DLCs to the given path.
// Signature uses Game struct, not gameID.
func DownloadGameFiles(accessToken string, game Game, downloadPath string,
	gameLanguage string, platformName string, extrasFlag bool, dlcFlag bool, resumeFlag bool,
	flattenFlag bool, numThreads int,
) error {
	// HTTP clients
	httpClient := &http.Client{Timeout: 60 * time.Second} // Client follows redirects (for HEAD and GET)
	httpClientNoRedirect := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Prevent auto-redirects
		},
	}

	if err := createDirIfNotExist(downloadPath); err != nil {
		log.Error().Err(err).Str("path", downloadPath).Msg("Failed to create base download directory")
		return err
	}

	tasks := make(chan downloadTask, numThreads*2) // Buffer tasks slightly
	var wg sync.WaitGroup

	log.Info().Int("threads", numThreads).Msg("Starting download workers")
	// Start worker goroutines
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			log.Debug().Int("worker", workerID).Msg("Download worker started")
			for task := range tasks {
				log.Debug().Int("worker", workerID).Str("url", task.url).Msg("Worker received task")
				if err := processDownloadTask(task, accessToken, downloadPath, game.Title, httpClient, httpClientNoRedirect); err != nil {
					// Log error, but continue processing other tasks
					log.Error().Err(err).Str("filename", task.fileName).Str("url", task.url).Int("worker", workerID).Msg("Download task failed")
					fmt.Printf("Error downloading %s: %v\n", task.fileName, err)
				}
			}
			log.Debug().Int("worker", workerID).Msg("Download worker finished")
		}(i)
	}

	// --- Enqueue tasks ---
	log.Info().Str("game", game.Title).Msg("Queueing download tasks")

	// Helper to get decoded base filename from ManualURL
	getDecodedBaseFilename := func(manualURL string) string {
		fname := filepath.Base(manualURL)
		decoded, err := url.QueryUnescape(fname)
		if err != nil {
			log.Warn().Err(err).Str("filename", fname).Msg("Failed to URL decode filename, using original")
			return fname
		}
		return decoded
	}

	// Enqueue tasks for main game downloads
	queuedGameFiles := 0
	for _, download := range game.Downloads {
		if !strings.EqualFold(download.Language, gameLanguage) {
			log.Debug().Str("language", download.Language).Msg("Skipping game download language")
			continue
		}
		platformGroups := []struct {
			files  []PlatformFile
			subDir string
		}{
			{download.Platforms.Windows, "windows"},
			{download.Platforms.Mac, "mac"},
			{download.Platforms.Linux, "linux"},
		}
		for _, group := range platformGroups {
			if platformName != "all" && !strings.EqualFold(platformName, group.subDir) {
				log.Debug().Str("platform", group.subDir).Msg("Skipping game download platform")
				continue
			}
			for _, file := range group.files {
				if file.ManualURL == nil || *file.ManualURL == "" {
					log.Warn().Str("game", game.Title).Str("platform", group.subDir).Str("name", file.Name).Msg("Missing ManualURL for game file")
					continue
				}
				manualURL := *file.ManualURL
				// *** USE embed.gog.com ***
				if !strings.HasPrefix(manualURL, "http") {
					manualURL = fmt.Sprintf("https://embed.gog.com%s", manualURL)
				}
				tasks <- downloadTask{
					url:      manualURL,
					fileName: getDecodedBaseFilename(*file.ManualURL), // Use helper
					subDir:   group.subDir,
					resume:   resumeFlag,
					flatten:  flattenFlag,
				}
				queuedGameFiles++
			}
		}
	}
	log.Info().Int("count", queuedGameFiles).Msg("Queued main game files")

	// Enqueue tasks for game extras
	queuedExtras := 0
	if extrasFlag {
		for _, extra := range game.Extras {
			if extra.ManualURL == "" {
				log.Warn().Str("game", game.Title).Str("name", extra.Name).Msg("Missing ManualURL for extra")
				continue
			}
			manualURL := extra.ManualURL
			// *** USE embed.gog.com ***
			if !strings.HasPrefix(manualURL, "http") {
				manualURL = fmt.Sprintf("https://embed.gog.com%s", manualURL)
			}
			tasks <- downloadTask{
				url:      manualURL,
				fileName: getDecodedBaseFilename(extra.ManualURL), // Use helper
				subDir:   "extras",
				resume:   resumeFlag,
				flatten:  flattenFlag,
			}
			queuedExtras++
		}
	}
	log.Info().Int("count", queuedExtras).Msg("Queued main game extras")

	// Enqueue tasks for DLCs
	queuedDLCFiles := 0
	queuedDLCExtras := 0
	if dlcFlag {
		for _, dlc := range game.DLCs {
			log.Info().Str("dlc", dlc.Title).Msg("Queueing tasks for DLC")
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, gameLanguage) {
					log.Debug().Str("dlc", dlc.Title).Str("language", download.Language).Msg("Skipping DLC download language")
					continue
				}
				platformGroups := []struct {
					files  []PlatformFile
					subDir string
				}{
					{download.Platforms.Windows, "windows"},
					{download.Platforms.Mac, "mac"},
					{download.Platforms.Linux, "linux"},
				}
				for _, group := range platformGroups {
					if platformName != "all" && !strings.EqualFold(platformName, group.subDir) {
						log.Debug().Str("dlc", dlc.Title).Str("platform", group.subDir).Msg("Skipping DLC download platform")
						continue
					}
					for _, file := range group.files {
						if file.ManualURL == nil || *file.ManualURL == "" {
							log.Warn().Str("dlc", dlc.Title).Str("platform", group.subDir).Str("name", file.Name).Msg("Missing ManualURL for DLC file")
							continue
						}
						manualURL := *file.ManualURL
						// *** USE embed.gog.com ***
						if !strings.HasPrefix(manualURL, "http") {
							manualURL = fmt.Sprintf("https://embed.gog.com%s", manualURL)
						}
						dlcSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), group.subDir)
						tasks <- downloadTask{
							url:      manualURL,
							fileName: getDecodedBaseFilename(*file.ManualURL), // Use helper
							subDir:   dlcSubDir,
							resume:   resumeFlag,
							flatten:  flattenFlag,
						}
						queuedDLCFiles++
					}
				}
			}
			// Enqueue DLC extras
			if extrasFlag {
				for _, extra := range dlc.Extras {
					if extra.ManualURL == "" {
						log.Warn().Str("dlc", dlc.Title).Str("name", extra.Name).Msg("Missing ManualURL for DLC extra")
						continue
					}
					manualURL := extra.ManualURL
					// *** USE embed.gog.com ***
					if !strings.HasPrefix(manualURL, "http") {
						manualURL = fmt.Sprintf("https://embed.gog.com%s", manualURL)
					}
					dlcExtraSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), "extras")
					tasks <- downloadTask{
						url:      manualURL,
						fileName: getDecodedBaseFilename(extra.ManualURL), // Use helper
						subDir:   dlcExtraSubDir,
						resume:   resumeFlag,
						flatten:  flattenFlag,
					}
					queuedDLCExtras++
				}
			}
		}
	}
	log.Info().Int("files", queuedDLCFiles).Int("extras", queuedDLCExtras).Msg("Queued DLC files and extras")

	// Signal that no more tasks will be sent
	close(tasks)
	log.Info().Msg("All download tasks queued. Waiting for workers to finish.")

	// Wait for all worker goroutines to complete
	wg.Wait()
	log.Info().Str("game", game.Title).Msg("All download workers finished.")

	// Save game metadata (even if downloads failed, metadata might be useful)
	metadata, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		log.Error().Err(err).Str("game", game.Title).Msg("Failed to marshal game metadata")
		// Consider returning this error if critical
	} else {
		// Save metadata in the root of the sanitized game directory
		metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
		if err = createDirIfNotExist(filepath.Dir(metadataPath)); err != nil { // Assign error
			log.Error().Err(err).Str("path", filepath.Dir(metadataPath)).Msg("Failed to create directory for metadata file")
		} else if err = os.WriteFile(metadataPath, metadata, 0o644); err != nil { // Assign error
			log.Error().Err(err).Str("path", metadataPath).Msg("Failed to save game metadata")
			// Don't return error here either
		} else {
			log.Info().Str("path", metadataPath).Msg("Game metadata saved successfully")
		}
	}

	return nil // Consider returning an aggregated error if any task failed
}
