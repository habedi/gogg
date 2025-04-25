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
	fileName string
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
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// GOG uses 302 Found for download redirects
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			log.Debug().Str("from", urlStr).Str("to", location).Msg("Download URL redirected")
			return location, nil
		}
		log.Warn().Str("url", urlStr).Msg("Redirect status received but Location header is missing")
	} else if resp.StatusCode != http.StatusOK {
		log.Warn().Str("url", urlStr).Int("status", resp.StatusCode).Msg("Unexpected status code when checking for redirect")
	}
	return "", nil // No redirect found or needed
}

// processDownloadTask handles a single download task.
func processDownloadTask(task downloadTask, accessToken, downloadPath, gameTitle string, client, clientNoRedirect *http.Client) error {
	urlStr := task.url
	fileName := task.fileName
	subDir := task.subDir
	resume := task.resume
	flatten := task.flatten

	log.Debug().Str("url", urlStr).Str("original_filename", fileName).Msg("Processing download task")

	// Check for redirect location, update URL and potentially filename if redirected.
	if redirectURL, err := findRedirectLocation(accessToken, urlStr, clientNoRedirect); err == nil && redirectURL != "" {
		log.Info().Str("from", urlStr).Str("to", redirectURL).Msg("Following download redirect")
		urlStr = redirectURL
		// Extract filename from the redirected URL as it might be different
		if u, err := url.Parse(redirectURL); err == nil {
			if base := filepath.Base(u.Path); base != "." && base != "/" {
				fileName = base
				log.Debug().Str("filename", fileName).Msg("Updated filename from redirect URL")
			}
		}
	} else if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("Failed to check for redirect")
		// Proceed with original URL, might still work
	}

	decodedFileName, err := url.QueryUnescape(fileName)
	if err != nil {
		log.Warn().Err(err).Str("filename", fileName).Msg("Failed to URL decode filename, using original")
		// Use the potentially encoded filename if decoding fails
	} else {
		fileName = decodedFileName
	}
	log.Debug().Str("filename", fileName).Msg("Using decoded filename")

	// Determine final subdirectory path
	if flatten {
		subDir = "" // No subdirectory if flattening
	} else {
		subDir = SanitizePath(subDir) // Sanitize subdirectory name if not flattening
	}

	gameDir := SanitizePath(gameTitle)
	finalDirPath := filepath.Join(downloadPath, gameDir, subDir)
	filePath := filepath.Join(finalDirPath, fileName)

	if err := createDirIfNotExist(finalDirPath); err != nil {
		log.Error().Err(err).Str("path", finalDirPath).Msg("Failed to create directory")
		return err
	}

	var file *os.File
	var startOffset int64 = 0

	// Handle file creation/resume logic
	if resume {
		if fileInfo, err := os.Stat(filePath); err == nil {
			startOffset = fileInfo.Size()
			log.Info().Str("file", fileName).Int64("offset", startOffset).Msg("Resuming download")
			file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
			if err != nil {
				log.Error().Err(err).Str("path", filePath).Msg("Failed to open file for appending")
				return err
			}
		} else if os.IsNotExist(err) {
			log.Info().Str("file", fileName).Msg("Starting new download (resume enabled, file not found)")
			file, err = os.Create(filePath)
			if err != nil {
				log.Error().Err(err).Str("path", filePath).Msg("Failed to create file")
				return err
			}
		} else {
			// Other error (e.g., permissions)
			log.Error().Err(err).Str("path", filePath).Msg("Failed to stat file for resume")
			return err
		}
	} else {
		log.Info().Str("file", fileName).Msg("Starting new download (resume disabled)")
		file, err = os.Create(filePath)
		if err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Failed to create file")
			return err
		}
	}
	defer file.Close()

	// Use HEAD request to get total file size for progress bar and completion check
	headReq, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Failed to create HEAD request")
		// Continue without HEAD, progress bar might be indeterminate
	}
	headReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	headResp, err := client.Do(headReq) // Use the client that follows redirects for HEAD
	var totalSize int64 = -1            // Default to unknown size
	if err != nil {
		log.Warn().Err(err).Str("url", urlStr).Msg("HEAD request failed")
	} else {
		headResp.Body.Close()
		if headResp.StatusCode == http.StatusOK {
			totalSize = headResp.ContentLength
			log.Debug().Str("file", fileName).Int64("size", totalSize).Msg("Got file size via HEAD")
		} else {
			log.Warn().Str("url", urlStr).Int("status", headResp.StatusCode).Msg("Unexpected status code from HEAD request")
		}
	}

	// Check if file is already complete
	if resume && totalSize > 0 && startOffset == totalSize {
		log.Info().Str("file", fileName).Msg("File already fully downloaded. Skipping.")
		fmt.Printf("Skipping %s (already complete).\n", fileName)
		return nil
	}
	if resume && totalSize > 0 && startOffset > totalSize {
		log.Warn().Str("file", fileName).Int64("offset", startOffset).Int64("totalSize", totalSize).Msg("Local file larger than remote size. Restarting download.")
		startOffset = 0
		// Truncate the file before restarting
		if err := file.Truncate(0); err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Failed to truncate oversized file")
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			log.Error().Err(err).Str("path", filePath).Msg("Failed to seek to start after truncate")
			return err
		}
	}

	// Prepare the GET request
	getReq, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("Failed to create GET request")
		return err
	}
	getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if resume && startOffset > 0 {
		getReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		log.Debug().Str("file", fileName).Int64("offset", startOffset).Msg("Added Range header")
	}

	// Execute the GET request using the redirect-following client
	getResp, err := client.Do(getReq)
	if err != nil {
		log.Error().Err(err).Str("url", urlStr).Msg("GET request failed")
		return err
	}
	defer getResp.Body.Close()

	// Check response status for download
	if getResp.StatusCode != http.StatusOK && getResp.StatusCode != http.StatusPartialContent {
		err = fmt.Errorf("unexpected HTTP status: %s", getResp.Status)
		log.Error().Err(err).Str("url", urlStr).Int("status", getResp.StatusCode).Msg("Download request failed")
		return err
	}

	// If totalSize wasn't determined by HEAD, try ContentLength from GET response
	if totalSize < 0 {
		totalSize = getResp.ContentLength
		if totalSize > 0 {
			log.Debug().Str("file", fileName).Int64("size", totalSize).Msg("Got file size via GET ContentLength")
			// If resuming, add the offset back to get the true total size
			if resume && startOffset > 0 {
				totalSize += startOffset
			}
		} else {
			log.Warn().Str("file", fileName).Msg("Could not determine file size. Progress bar may be inaccurate.")
		}
	}

	// Setup progress bar
	barDescription := fmt.Sprintf("Downloading %s", fileName)
	// Truncate description if too long for terminal display
	maxDescLen := 50
	if len(barDescription) > maxDescLen {
		barDescription = barDescription[:maxDescLen-3] + "..."
	}

	progressBar := progressbar.NewOptions64(
		totalSize, // Use totalSize determined earlier, might be -1
		progressbar.OptionSetDescription(barDescription),
		progressbar.OptionSetWidth(40),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "#",
			SaucerPadding: " ",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionSetPredictTime(false),
		// progressbar.OptionClearOnFinish(), // Removed this line
		progressbar.OptionThrottle(100*time.Millisecond), // Refresh rate
		progressbar.OptionSpinnerType(14),                // Choose a spinner type
		// Use unknown size indicator if totalSize is -1
		progressbar.OptionSetVisibility(totalSize >= 0),
	)

	// Set initial progress for resuming downloads
	if err := progressBar.Set64(startOffset); err != nil {
		log.Warn().Err(err).Msg("Failed to set progress bar offset")
		// Continue without setting offset
	}

	// Create a reader that updates the progress bar
	progressReader := progressbar.NewReader(getResp.Body, progressBar)

	// Copy data from response body to file via progress reader
	written, err := io.Copy(file, &progressReader)
	if err != nil {
		// io.Copy might return an error, but partial data might have been written
		log.Error().Err(err).Str("file", fileName).Msg("Error during download stream copy")
		// Attempt to clear the progress bar line on error
		progressBar.Clear()
		return fmt.Errorf("failed to save file %s: %w", filePath, err)
	}
	log.Info().Str("file", fileName).Int64("bytes", written).Msg("Download stream finished")

	// Final check: if resuming and the total bytes don't match, log warning
	if totalSize > 0 && (startOffset+written) != totalSize {
		log.Warn().Str("file", fileName).
			Int64("expected", totalSize).
			Int64("actual", startOffset+written).
			Msg("Downloaded size does not match expected size")
	}

	progressBar.Clear() // Ensure the bar is cleared on success too
	fmt.Printf("Finished downloading %s\n", fileName)
	return nil
}

// DownloadGameFiles downloads game files, extras, and DLCs to the given path.
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

	// Enqueue tasks for main game downloads
	queuedGameFiles := 0
	for _, download := range game.Downloads {
		if !strings.EqualFold(download.Language, gameLanguage) {
			log.Debug().Str("language", download.Language).Msg("Skipping game download language")
			continue
		}
		platformGroups := []struct {
			files  []PlatformFile
			subDir string // Relative sub-directory name (e.g., "windows")
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
				rawURL := *file.ManualURL
				// Prepend base URL if necessary (GOG often uses relative paths)
				if !strings.HasPrefix(rawURL, "http") {
					// This base URL might need verification, but seems plausible for installers
					rawURL = "https://content-system.gog.com" + rawURL
				}
				tasks <- downloadTask{
					url:      rawURL,
					fileName: filepath.Base(rawURL), // Initial filename guess
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
			rawURL := extra.ManualURL
			if !strings.HasPrefix(rawURL, "http") {
				rawURL = "https://content-system.gog.com" + rawURL
			}
			tasks <- downloadTask{
				url:      rawURL,
				fileName: filepath.Base(rawURL),
				subDir:   "extras", // Standard sub-directory for extras
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
						rawURL := *file.ManualURL
						if !strings.HasPrefix(rawURL, "http") {
							rawURL = "https://content-system.gog.com" + rawURL
						}
						// Place DLCs in a structured path like 'dlcs/<dlc_title>/<platform>'
						dlcSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), group.subDir)
						tasks <- downloadTask{
							url:      rawURL,
							fileName: filepath.Base(rawURL),
							subDir:   dlcSubDir,
							resume:   resumeFlag,
							flatten:  flattenFlag, // Flatten applies relative to game dir
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
					rawURL := extra.ManualURL
					if !strings.HasPrefix(rawURL, "http") {
						rawURL = "https://content-system.gog.com" + rawURL
					}
					dlcExtraSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), "extras")
					tasks <- downloadTask{
						url:      rawURL,
						fileName: filepath.Base(rawURL),
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
		// Don't return error here, downloads might have partially succeeded
	} else {
		// Save metadata in the root of the sanitized game directory
		metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
		if err := createDirIfNotExist(filepath.Dir(metadataPath)); err != nil {
			log.Error().Err(err).Str("path", filepath.Dir(metadataPath)).Msg("Failed to create directory for metadata file")
		} else if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
			log.Error().Err(err).Str("path", metadataPath).Msg("Failed to save game metadata")
			// Don't return error here either
		} else {
			log.Info().Str("path", metadataPath).Msg("Game metadata saved successfully")
		}
	}

	return nil // Consider returning an aggregated error if any task failed
}
