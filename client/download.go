package client

import (
	"context" // Import context
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	netURL "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time" // Import time

	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
)

// ParseGameData remains the same...
func ParseGameData(data string) (Game, error) {
	var rawResponse Game
	if err := json.Unmarshal([]byte(data), &rawResponse); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return Game{}, err
	}
	return rawResponse, nil
}

// ensureDirExists remains the same...
func ensureDirExists(path string) error {
	// ... (implementation unchanged) ...
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			log.Error().Msgf("Path %s exists but is not a directory", path)
			return fmt.Errorf("path %s exists but is not a directory", path) // Return specific error
		}
		return nil
	}
	if os.IsNotExist(err) {
		log.Info().Msgf("Creating directory: %s", path)
		return os.MkdirAll(path, os.ModePerm)
	}
	log.Error().Err(err).Msgf("Error checking directory %s", path)
	return err
}

// SanitizePath remains the same...
func SanitizePath(name string) string {
	// ... (implementation unchanged) ...
	replacements := []struct {
		old string
		new string
	}{
		{"®", ""}, {":", ""}, {" ", "-"}, {"(", ""}, {")", ""}, {"™", ""}, // Added ™
	}
	name = strings.ToLower(name)
	for _, r := range replacements {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	// Remove any remaining characters not suitable for file paths if needed
	// name = regexp.MustCompile(`[^a-z0-9-_]+`).ReplaceAllString(name, "")
	return name
}

// downloadTask remains the same...
type downloadTask struct {
	url      string
	fileName string
	subDir   string
	resume   bool
	flatten  bool
}

// DownloadGameFiles downloads the game files, extras, and DLCs to the specified path.
// ADDED: ctx context.Context, progressWriter io.Writer
func DownloadGameFiles(
	ctx context.Context, // Add context parameter
	accessToken string, game Game, downloadPath string,
	gameLanguage string, platformName string, extrasFlag bool, dlcFlag bool, resumeFlag bool,
	flattenFlag bool, skipPatchesFlag bool, numThreads int,
	progressWriter io.Writer, // Add writer parameter
) error {
	client := &http.Client{} // Standard client for most requests
	// Client specifically for finding redirect location (doesn't follow redirects)
	clientNoRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if err := ensureDirExists(downloadPath); err != nil {
		log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
		return err
	}

	// findFileLocation checks for redirects. Now checks context.
	findFileLocation := func(ctx context.Context, url string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Use context
		if err != nil {
			log.Error().Err(err).Msg("Failed to create request for redirect check")
			return "", err // Return error
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, err := clientNoRedirect.Do(req)
		if err != nil {
			// Check if context was cancelled
			if ctx.Err() == context.Canceled {
				log.Info().Msg("Redirect check cancelled.")
				return "", ctx.Err()
			}
			log.Error().Err(err).Msg("Failed to send request for redirect check")
			return "", err // Return error
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently ||
			resp.StatusCode == http.StatusTemporaryRedirect || resp.StatusCode == http.StatusPermanentRedirect {
			location := resp.Header.Get("Location")
			if location != "" {
				log.Info().Msgf("Redirected to: %s", location)
				return location, nil // Return location
			}
			log.Warn().Msg("Redirect status found but Location header missing")
			return "", fmt.Errorf("redirect location not found in header") // Return error
		} else if resp.StatusCode != http.StatusOK { // Handle other non-redirect, non-ok statuses
			log.Warn().Msgf("Redirect check: Non-redirect response status: %d for %s", resp.StatusCode, url)
			// Optionally return an error here if needed, or just return ""
			// return "", fmt.Errorf("unexpected status %d during redirect check", resp.StatusCode)
		}
		// If no redirect, return original url or empty string? Let's return empty string + nil error
		return "", nil // No redirect found, not an error
	}

	// downloadFile performs the actual download for a single task. Now checks context.
	downloadFile := func(ctx context.Context, task downloadTask) error {
		// Check for cancellation at the start of the task
		select {
		case <-ctx.Done():
			log.Info().Msgf("Download task for %s cancelled before starting.", task.fileName)
			return ctx.Err()
		default:
			// Continue
		}

		url := task.url
		fileName := task.fileName
		subDir := task.subDir
		resume := task.resume
		flatten := flattenFlag

		// Find redirect location
		location, err := findFileLocation(ctx, url)
		if err != nil {
			// Handle context cancellation error specifically
			if err == context.Canceled || err == context.DeadlineExceeded {
				log.Info().Msgf("Download cancelled during redirect check for %s", fileName)
				return err
			}
			// Log other redirect check errors but maybe try original URL? Or fail? Let's fail.
			log.Error().Err(err).Msgf("Failed redirect check for %s", url)
			return fmt.Errorf("failed redirect check for %s: %w", url, err)
		}
		if location != "" {
			url = location
			// Update filename based on redirected URL only if it has path component
			if parsedLoc, parseErr := netURL.Parse(location); parseErr == nil && parsedLoc.Path != "" {
				base := filepath.Base(parsedLoc.Path)
				if base != "." && base != "/" {
					fileName = base
				}
			}
		}

		// Decode URL-encoded characters in the file name
		decodedFileName, err := netURL.QueryUnescape(fileName)
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to decode file name %s, using original", fileName)
			// Continue with the potentially encoded filename instead of failing
		} else {
			fileName = decodedFileName
		}

		// Prepare file path
		if flatten {
			subDir = ""
		} else {
			subDir = SanitizePath(subDir)
		}
		gameTitle := SanitizePath(game.Title)
		// Ensure the base download path exists first (should be done outside this func)
		// Then create the specific directory for this file
		targetDir := filepath.Join(downloadPath, gameTitle, subDir)
		filePath := filepath.Join(targetDir, fileName)

		if err := ensureDirExists(targetDir); err != nil {
			log.Error().Err(err).Msgf("Failed to prepare directory for %s", filePath)
			return err
		}

		var file *os.File
		var startOffset int64

		// Handle resume logic
		if resume {
			fileInfo, err := os.Stat(filePath)
			if err == nil { // File exists
				startOffset = fileInfo.Size()
				log.Info().Msgf("Resuming download for %s from offset %d", fileName, startOffset)
				file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to open file %s for appending", filePath)
					return err
				}
			} else if os.IsNotExist(err) { // File doesn't exist
				file, err = os.Create(filePath)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to create file %s", filePath)
					return err
				}
			} else { // Other stat error
				log.Error().Err(err).Msgf("Failed to stat file %s", filePath)
				return err
			}
		} else { // Not resuming, always create/truncate
			file, err = os.Create(filePath)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to create file %s", filePath)
				return err
			}
		}
		defer file.Close()

		// HEAD request to get total size (check context)
		headReq, err := http.NewRequestWithContext(ctx, "HEAD", url, nil) // Use context
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create HEAD request for %s", fileName)
			return err
		}
		headReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		headResp, err := client.Do(headReq)
		if err != nil {
			if ctx.Err() == context.Canceled {
				log.Info().Msgf("HEAD request cancelled for %s.", fileName)
				return ctx.Err()
			}
			log.Error().Err(err).Msgf("Failed HEAD request for %s", fileName)
			return err
		}
		headResp.Body.Close() // Close body even for HEAD

		if headResp.StatusCode != http.StatusOK {
			// Maybe retry? For now, log and fail.
			log.Error().Msgf("HEAD request failed for %s: HTTP %d", fileName, headResp.StatusCode)
			// Attempting GET anyway might work if HEAD isn't supported, but could lead to unknown total size.
			// Let's require HEAD to succeed for robust resume/progress.
			return fmt.Errorf("failed to get file info for %s: HTTP %d", fileName, headResp.StatusCode)
		}
		totalSize := headResp.ContentLength
		if totalSize <= 0 {
			log.Warn().Msgf("Content-Length for %s is %d, progress/resume might be inaccurate.", fileName, totalSize)
			totalSize = -1 // Indicate unknown size for progress bar
		}

		// Check if file is already fully downloaded (only if size is known)
		if resume && totalSize > 0 && startOffset >= totalSize {
			log.Info().Msgf("File %s is already fully downloaded (%d bytes). Skipping download.", fileName, totalSize)
			// Use progressWriter to inform GUI
			fmt.Fprintf(progressWriter, "Skipping already downloaded file: %s\n", fileName)
			return nil
		}

		// GET request (check context)
		getReq, err := http.NewRequestWithContext(ctx, "GET", url, nil) // Use context
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create GET request for %s", fileName)
			return err
		}
		getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		if resume && startOffset > 0 {
			getReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		}

		getResp, err := client.Do(getReq)
		if err != nil {
			if ctx.Err() == context.Canceled {
				log.Info().Msgf("GET request cancelled for %s.", fileName)
				return ctx.Err()
			}
			log.Error().Err(err).Msgf("Failed to download %s", fileName)
			return err
		}
		defer getResp.Body.Close()

		// Check response status code (200 OK or 206 Partial Content)
		if getResp.StatusCode != http.StatusOK && getResp.StatusCode != http.StatusPartialContent {
			log.Error().Msgf("Failed to download %s: HTTP %d", fileName, getResp.StatusCode)
			return fmt.Errorf("failed to download %s: HTTP %d", fileName, getResp.StatusCode)
		}

		// If resuming, verify Content-Range header if possible (optional but good)

		// Initialize progress bar, writing to the custom writer
		progressBar := progressbar.NewOptions64(
			totalSize, // Will handle -1 correctly (shows spinner)
			progressbar.OptionSetDescription(fmt.Sprintf("Downloading %s", fileName)),
			// progressbar.OptionSetWidth(40), // Width might not be relevant for log output
			progressbar.OptionShowBytes(true),
			// progressbar.OptionSetTheme(progressbar.Theme{...}), // Theme chars likely bad for log
			progressbar.OptionSetWriter(progressWriter),      // Use the passed writer
			progressbar.OptionThrottle(500*time.Millisecond), // Throttle updates to GUI
			progressbar.OptionClearOnFinish(),                // Clear the progress line on finish
			progressbar.OptionSpinnerType(14),                // Use a simple spinner if size unknown
			// We need to show KiB/MiB/GiB based on size dynamically
			progressbar.OptionSetPredictTime(false), // Predict time might be noisy
			progressbar.OptionShowCount(),           // Show count might be useful
		)
		// Set the progress bar to the starting point (resume offset)
		if startOffset > 0 {
			if err := progressBar.Set64(startOffset); err != nil {
				log.Warn().Err(err).Msg("failed to set progress start offset")
			}
		}

		// Wrap response body with progress bar reader
		// progressReader := io.TeeReader(getResp.Body, progressBar) // TeeReader might buffer unnecessarily
		progressReader := progressbar.NewReader(getResp.Body, progressBar)

		// Write the downloaded content to the file
		// Use io.CopyBuffer to potentially reduce allocations and check context during copy
		buffer := make([]byte, 32*1024) // 32KB buffer
		_, err = io.CopyBuffer(file, &progressReader, buffer)
		if err != nil {
			// Check context cancellation first
			if ctx.Err() == context.Canceled {
				log.Info().Msgf("Download cancelled while copying %s", fileName)
				// Attempt to remove partially downloaded file on cancel? Or rely on resume?
				// Let's rely on resume for now.
				return ctx.Err()
			}
			// Handle other copy errors
			log.Error().Err(err).Msgf("Failed to save file content for %s", filePath)
			// Attempt to remove partial file on error
			_ = os.Remove(filePath)
			return fmt.Errorf("failed to save file %s: %w", filePath, err)
		}

		// Explicitly finish progress bar to ensure final output/clearance
		_ = progressBar.Finish()
		fmt.Fprintf(progressWriter, "Finished downloading: %s\n", fileName) // Add final message

		return nil
	} // End of downloadFile function

	// Create a channel to distribute download tasks
	taskChan := make(chan downloadTask, numThreads*2) // Buffer size based on threads
	var wg sync.WaitGroup
	var downloadErrors sync.Map // Use sync.Map to safely store errors from multiple goroutines

	// Start worker goroutines
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			log.Debug().Msgf("Starting download worker %d", workerID)
			for {
				select {
				case <-ctx.Done(): // Check for cancellation before taking a task
					log.Debug().Msgf("Download worker %d exiting due to cancellation.", workerID)
					return
				case task, ok := <-taskChan:
					if !ok { // Channel closed
						log.Debug().Msgf("Download worker %d exiting as channel closed.", workerID)
						return
					}
					log.Debug().Msgf("Worker %d processing task: %s", workerID, task.fileName)
					if err := downloadFile(ctx, task); err != nil {
						// Check if error is due to cancellation
						if err == context.Canceled || err == context.DeadlineExceeded {
							log.Info().Msgf("Task for %s cancelled.", task.fileName)
							// Store cancellation error? Optional.
							downloadErrors.Store(task.fileName, err) // Store error associated with the file
						} else {
							log.Error().Err(err).Msgf("Worker %d failed to download %s", workerID, task.fileName)
							downloadErrors.Store(task.fileName, err) // Store error
						}
						// Even on error, continue processing next task unless cancelled
					}
				}
			}
		}(i)
	}

	// Enqueue download tasks, checking for cancellation
	enqueueTasks := func() error {
		defer close(taskChan) // Close channel when all tasks are enqueued or cancelled

		// Enqueue game files
		for _, download := range game.Downloads {
			// Check context before processing each language block
			select {
			case <-ctx.Done():
				log.Info().Msg("Download cancelled while enqueueing game files.")
				return ctx.Err()
			default:
			}

			if !strings.EqualFold(download.Language, gameLanguage) { // Use EqualFold for case-insensitivity
				log.Info().Msgf("Skipping game download for language %s (selected: %s)", download.Language, gameLanguage)
				fmt.Fprintf(progressWriter, "Skipping game files for language: %s\n", download.Language)
				continue
			}

			for _, platformFiles := range []struct {
				files  []PlatformFile
				subDir string
			}{
				{download.Platforms.Windows, "windows"},
				{download.Platforms.Mac, "mac"},
				{download.Platforms.Linux, "linux"},
			} {
				if platformName != "all" && !strings.EqualFold(platformName, platformFiles.subDir) {
					log.Info().Msgf("Skipping game download for platform %s (selected: %s)", platformFiles.subDir, platformName)
					fmt.Fprintf(progressWriter, "Skipping game files for platform: %s\n", platformFiles.subDir)
					continue
				}

				for _, file := range platformFiles.files {
					if file.ManualURL == nil || *file.ManualURL == "" {
						log.Warn().Msgf("Skipping file with nil or empty ManualURL in game %s, platform %s", game.Title, platformFiles.subDir)
						continue
					}
					if strings.Contains(*file.ManualURL, "en1patch") {
						fmt.Fprintf(progressWriter, "Skipping patch in game %s, platform %s, file name %s\n", game.Title, platformFiles.subDir, *file.ManualURL)
						continue
					}
					url := fmt.Sprintf("https://embed.gog.com%s", *file.ManualURL)
					// Use ManualURL as base for filename if possible, fallback needed?
					fileName := filepath.Base(*file.ManualURL)
					if fileName == "." || fileName == "/" {
						fileName = fmt.Sprintf("download_%s", file.Name) // Fallback filename
						log.Warn().Msgf("Could not determine filename from ManualURL '%s', using fallback '%s'", *file.ManualURL, fileName)
					}
					task := downloadTask{
						url, fileName, platformFiles.subDir,
						resumeFlag, flattenFlag,
					}
					// Send task, checking context
					select {
					case taskChan <- task:
					case <-ctx.Done():
						log.Info().Msg("Download cancelled while enqueueing game file task.")
						return ctx.Err()
					}
				}
			}
		}

		// Enqueue extras
		if extrasFlag {
			for _, extra := range game.Extras {
				select {
				case <-ctx.Done():
					log.Info().Msg("Download cancelled while enqueueing extras.")
					return ctx.Err()
				default:
				}
				if extra.ManualURL == "" {
					log.Warn().Msgf("Skipping extra '%s' with empty ManualURL", extra.Name)
					continue
				}
				extraURL := fmt.Sprintf("https://embed.gog.com%s", extra.ManualURL)
				// Use extra name for subdir/filename if possible
				extraFileName := SanitizePath(extra.Name)
				if ext := filepath.Ext(extra.ManualURL); ext != "" {
					extraFileName += ext // Append original extension
				}
				if extraFileName == "" {
					extraFileName = filepath.Base(extra.ManualURL) // Fallback
				}
				task := downloadTask{
					extraURL, extraFileName, "extras",
					resumeFlag, flattenFlag,
				}
				select {
				case taskChan <- task:
				case <-ctx.Done():
					log.Info().Msg("Download cancelled while enqueueing extra task.")
					return ctx.Err()
				}
			}
		} else {
			fmt.Fprintf(progressWriter, "Skipping all extras.\n")
		}

		// Enqueue DLCs
		if dlcFlag {
			for _, dlc := range game.DLCs {
				log.Info().Msgf("Processing DLC: %s", dlc.Title)
				fmt.Fprintf(progressWriter, "Processing DLC: %s\n", dlc.Title)

				// Enqueue DLC files
				for _, download := range dlc.ParsedDownloads {
					select {
					case <-ctx.Done():
						log.Info().Msg("Download cancelled while enqueueing DLC files.")
						return ctx.Err()
					default:
					}
					if !strings.EqualFold(download.Language, gameLanguage) {
						log.Info().Msgf("Skipping DLC download for language %s (DLC: %s)", download.Language, dlc.Title)
						fmt.Fprintf(progressWriter, "Skipping DLC '%s' files for language: %s\n", dlc.Title, download.Language)
						continue
					}

					for _, platformFiles := range []struct {
						files  []PlatformFile
						subDir string
					}{
						{download.Platforms.Windows, "windows"},
						{download.Platforms.Mac, "mac"},
						{download.Platforms.Linux, "linux"},
					} {
						if platformName != "all" && !strings.EqualFold(platformName, platformFiles.subDir) {
							log.Info().Msgf("Skipping DLC download for platform %s (DLC: %s)", platformFiles.subDir, dlc.Title)
							fmt.Fprintf(progressWriter, "Skipping DLC '%s' files for platform: %s\n", dlc.Title, platformFiles.subDir)
							continue
						}

						for _, file := range platformFiles.files {
							if file.ManualURL == nil || *file.ManualURL == "" {
								log.Warn().Msgf("Skipping file with nil or empty ManualURL in DLC %s, platform %s", dlc.Title, platformFiles.subDir)
								continue
							}
							url := fmt.Sprintf("https://embed.gog.com%s", *file.ManualURL)
							fileName := filepath.Base(*file.ManualURL)
							if fileName == "." || fileName == "/" {
								fileName = fmt.Sprintf("dlc_%s_%s", SanitizePath(dlc.Title), file.Name) // Fallback
								log.Warn().Msgf("Could not determine filename from ManualURL '%s' for DLC, using fallback '%s'", *file.ManualURL, fileName)
							}
							// Construct subDir for DLCs, considering flatten flag
							dlcSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), platformFiles.subDir)

							task := downloadTask{
								url, fileName, dlcSubDir,
								resumeFlag, flattenFlag, // Pass flatten flag here
							}
							select {
							case taskChan <- task:
							case <-ctx.Done():
								log.Info().Msg("Download cancelled while enqueueing DLC file task.")
								return ctx.Err()
							}
						}
					}
				} // End DLC ParsedDownloads loop

				// Enqueue DLC extras
				if extrasFlag {
					for _, extra := range dlc.Extras {
						select {
						case <-ctx.Done():
							log.Info().Msg("Download cancelled while enqueueing DLC extras.")
							return ctx.Err()
						default:
						}
						if extra.ManualURL == "" {
							log.Warn().Msgf("Skipping extra '%s' for DLC '%s' with empty ManualURL", extra.Name, dlc.Title)
							continue
						}
						extraURL := fmt.Sprintf("https://embed.gog.com%s", extra.ManualURL)
						extraFileName := SanitizePath(extra.Name)
						if ext := filepath.Ext(extra.ManualURL); ext != "" {
							extraFileName += ext // Append original extension
						}
						if extraFileName == "" {
							extraFileName = filepath.Base(extra.ManualURL) // Fallback
						}
						dlcExtraSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title), "extras")

						task := downloadTask{
							extraURL, extraFileName,
							dlcExtraSubDir, resumeFlag, flattenFlag, // Pass flatten flag here
						}
						select {
						case taskChan <- task:
						case <-ctx.Done():
							log.Info().Msg("Download cancelled while enqueueing DLC extra task.")
							return ctx.Err()
						}
					}
				} // End DLC extras enqueue
			} // End DLC loop
		} else {
			fmt.Fprintf(progressWriter, "Skipping all DLCs.\n")
		}

		return nil // Enqueueing completed successfully
	}

	// Start enqueueing in a separate goroutine so we don't block wg.Wait()
	var enqueueErr error
	go func() {
		enqueueErr = enqueueTasks()
		if enqueueErr != nil {
			log.Error().Err(enqueueErr).Msg("Task enqueueing failed or was cancelled.")
			// Optionally signal workers to stop if not already handled by context?
			// Context cancellation should handle this.
		}
	}()

	// Wait for all worker goroutines to finish
	wg.Wait()
	log.Info().Msg("All download workers finished.")

	// Check if enqueueing itself was cancelled or errored
	if enqueueErr != nil {
		return enqueueErr // Return the cancellation/enqueueing error
	}

	// Check if any download tasks failed
	var firstError error
	errorCount := 0
	downloadErrors.Range(func(key, value interface{}) bool {
		fileName := key.(string)
		err := value.(error)
		if firstError == nil && err != context.Canceled && err != context.DeadlineExceeded { // Report first non-cancel error
			firstError = fmt.Errorf("failed to download %s: %w", fileName, err)
		}
		errorCount++
		log.Warn().Err(err).Msgf("Recorded error for file: %s", fileName)
		return true // Continue iterating
	})

	if errorCount > 0 {
		log.Warn().Msgf("Total download errors: %d", errorCount)
		// Return the first non-cancellation error encountered, or a generic error if only cancellations occurred
		if firstError != nil {
			return firstError
		} else {
			// Check if the main context was cancelled, indicating user action
			if ctx.Err() == context.Canceled {
				return ctx.Err() // Return context cancelled error
			}
			// Otherwise, maybe individual tasks were cancelled due to timeout?
			return fmt.Errorf("%d download tasks failed or were cancelled", errorCount)
		}
	}

	// Save metadata only if download completes successfully without cancellation
	metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
	// Check context before writing metadata
	select {
	case <-ctx.Done():
		log.Info().Msg("Download cancelled before saving metadata.")
		return ctx.Err()
	default:
		metadata, err := json.MarshalIndent(game, "", "  ")
		if err != nil {
			log.Error().Err(err).Msg("Failed to encode game metadata")
			// Don't return error here, download succeeded mostly
		} else {
			if err := ensureDirExists(filepath.Dir(metadataPath)); err == nil {
				if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
					log.Error().Err(err).Msgf("Failed to save game metadata to %s", metadataPath)
					// Don't return error here either
				} else {
					log.Info().Msgf("Saved metadata to %s", metadataPath)
				}
			} else {
				log.Error().Err(err).Msgf("Failed to create directory for metadata %s", metadataPath)
			}
		}
	}

	log.Info().Msg("Download process completed.")
	return nil // Success
}
