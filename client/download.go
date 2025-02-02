package client

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
	"io"
	"net/http"
	netURL "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ParseGameData parses the raw game data JSON into a Game struct.
// It takes a JSON string as input and returns a Game struct and an error if the parsing fails.
func ParseGameData(data string) (Game, error) {
	var rawResponse Game
	if err := json.Unmarshal([]byte(data), &rawResponse); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return Game{}, err
	}
	return rawResponse, nil
}

// ensureDirExists checks if a directory exists and creates it if it doesn't.
// It takes a directory path as input and returns an error if the directory cannot be created.
func ensureDirExists(path string) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			log.Error().Msgf("Path %s exists but is not a directory", path)
			return err
		}
		return nil
	}
	if os.IsNotExist(err) {
		return os.MkdirAll(path, os.ModePerm)
	}
	return err
}

// SanitizePath sanitizes a string to be used as a file path by removing special characters, spaces, and converting to lowercase.
// It takes a string as input and returns a sanitized string.
func SanitizePath(name string) string {
	replacements := []struct {
		old string
		new string
	}{
		{"®", ""},
		{":", ""},
		{" ", "-"},
		{"(", ""},
		{")", ""},
	}
	name = strings.ToLower(name)
	for _, r := range replacements {
		name = strings.ReplaceAll(name, r.old, r.new)
	}
	return name
}

// downloadTask represents a download task with URL, file name, sub-directory, resume, and flatten flags.
type downloadTask struct {
	url      string
	fileName string
	subDir   string
	resume   bool
	flatten  bool
}

// DownloadGameFiles downloads the game files, extras, and DLCs to the specified path.
// It takes various parameters including access token, game data, download path, language, platform, flags for extras, DLCs, resume, flatten, and number of threads.
// It returns an error if the download fails.
func DownloadGameFiles(accessToken string, game Game, downloadPath string,
	gameLanguage string, platformName string, extrasFlag bool, dlcFlag bool, resumeFlag bool,
	flattenFlag bool, numThreads int) error {
	client := &http.Client{}
	clientNoRedirect := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if err := ensureDirExists(downloadPath); err != nil {
		log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
		return err
	}

	findFileLocation := func(url string) string {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create request")
			return ""
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, err := clientNoRedirect.Do(req)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send request")
			return ""
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
			location := resp.Header.Get("Location")
			if location != "" {
				log.Info().Msgf("Redirected to: %s", location)
				return location
			}
			log.Info().Msg("Redirect location not found")
		} else {
			log.Info().Msgf("Response status: %d", resp.StatusCode)
		}
		return ""
	}

	downloadFiles := func(task downloadTask) error {

		// Error variable to store the error
		var err error

		url := task.url
		fileName := task.fileName
		subDir := task.subDir
		resume := task.resume
		flatten := flattenFlag

		if location := findFileLocation(url); location != "" {
			url = location
			fileName = filepath.Base(location)
		}

		// Decode URL-encoded characters in the file name (e.g. %20 -> space)
		decodedFileName, err := netURL.QueryUnescape(fileName)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to decode file name %s", fileName)
			return err
		}
		fileName = decodedFileName

		// Don't make additional directories if flatten is enabled
		if flatten {
			subDir = ""
		} else {
			subDir = SanitizePath(subDir)
		}

		// Sanitize the game title to be used as base directory name
		gameTitle := SanitizePath(game.Title)
		filePath := filepath.Join(downloadPath, gameTitle, subDir, fileName)

		if err := ensureDirExists(filepath.Dir(filePath)); err != nil {
			log.Error().Err(err).Msgf("Failed to prepare directory for %s", filePath)
			return err
		}

		// Check if resuming is enabled; and if file already exists and is fully downloaded
		var file *os.File
		var startOffset int64

		if resume {
			fileInfo, err := os.Stat(filePath)
			if err == nil {
				startOffset = fileInfo.Size()
				log.Info().Msgf("Resuming download for %s from offset %d", fileName, startOffset)
				file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to open file %s for appending", filePath)
					return err
				}
			} else if os.IsNotExist(err) {
				file, err = os.Create(filePath)
				if err != nil {
					log.Error().Err(err).Msgf("Failed to create file %s", filePath)
					return err
				}
			} else {
				log.Error().Err(err).Msgf("Failed to stat file %s", filePath)
				return err
			}
		} else {
			file, err = os.Create(filePath)
			if err != nil {
				log.Error().Err(err).Msgf("Failed to create file %s", filePath)
				return err
			}
		}
		defer file.Close()

		// First, make a HEAD request to get the total size of the file
		req, err := http.NewRequest("HEAD", url, nil)
		if err != nil {
			log.Error().Err(err).Msgf("Failed to create HEAD request for %s", fileName)
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		resp, err := client.Do(req)
		if err != nil {
			log.Error().Err(err).Msgf("failed to get file info for %s", fileName)
			return err
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Info().Msgf("HTTP %d", resp.StatusCode)
		}

		// Get the total size of the file to download
		totalSize := resp.ContentLength

		// Check if the file is already fully downloaded
		if startOffset >= totalSize {
			log.Info().Msgf("File %s is already fully downloaded (%d bytes). Skipping download.", fileName, totalSize)
			fmt.Printf("File %s is already fully downloaded. Skipping download.\n", fileName)
			return nil
		}

		// Add Range header if resuming
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			log.Error().Err(err).Msgf("failed to create GET request for %s", fileName)
			return err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		if resume && startOffset > 0 {
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
		}

		resp, err = client.Do(req)
		if err != nil {
			log.Error().Err(err).Msgf("failed to download %s", fileName)
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("failed to download %s: HTTP %d", fileName, resp.StatusCode)
		}

		// Initialize progress bar with starting point
		progressBar := progressbar.NewOptions64(
			totalSize,
			progressbar.OptionSetDescription(fmt.Sprintf("Downloading %s", fileName)),
			progressbar.OptionSetWidth(40),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "#",
				SaucerPadding: " ",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
		// Set progress bar to the starting point (resume offset)
		if err := progressBar.Set64(startOffset); err != nil {
			return fmt.Errorf("failed to set progress bar start offset: %w", err)
		}

		// Wrap response body with progress bar
		progressReader := io.TeeReader(resp.Body, progressBar)

		// Write the downloaded content to the file
		if _, err := io.Copy(file, progressReader); err != nil {
			return fmt.Errorf("failed to save file %s: %w", filePath, err)
		}

		return nil
	}

	// Create a channel to distribute download tasks
	taskChan := make(chan downloadTask, 10)
	var wg sync.WaitGroup

	// Start worker goroutines (lightweight threads) to download files concurrently
	for i := 0; i < numThreads; i++ { // Number of workers
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskChan {
				if err := downloadFiles(task); err != nil {
					log.Error().Err(err).Msgf("Failed to download %s", task.fileName)
				}
			}
		}()
	}

	// Enqueue download tasks
	for _, download := range game.Downloads {

		// Check if the language of the download matches the selected language
		if !strings.EqualFold(download.Language, gameLanguage) {
			log.Info().Msgf("Skipping download for language %s because it doesn't match selected language %s", download.Language, gameLanguage)
			fmt.Printf("Skipping downloading game files for %s\n", download.Language)
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
			for _, file := range platformFiles.files {
				if platformName != "all" && platformName != platformFiles.subDir {
					log.Info().Msgf("Skipping download for platform %s because it doesn't match selected platform %s", platformFiles.subDir, platformName)
					fmt.Printf("Skipping downloading game files for %s\n", platformFiles.subDir)
					continue
				}

				if file.ManualURL == nil {
					continue
				}

				url := fmt.Sprintf("https://embed.gog.com%s", *file.ManualURL)
				fileName := filepath.Base(*file.ManualURL)
				taskChan <- downloadTask{url, fileName, platformFiles.subDir,
					resumeFlag, flattenFlag}
			}
		}
	}

	for _, extra := range game.Extras {
		if !extrasFlag {
			log.Info().Msgf("Skipping downloadig extra %s", extra.Name)
			fmt.Printf("Skipping downloading extras for %s\n", extra.Name)
			continue
		}

		extraURL := fmt.Sprintf("https://embed.gog.com%s", extra.ManualURL)
		extraFileName := SanitizePath(extra.Name)
		taskChan <- downloadTask{extraURL, extraFileName, "extras",
			resumeFlag, flattenFlag}
	}

	if dlcFlag {
		for _, dlc := range game.DLCs {
			log.Info().Msgf("Processing DLC: %s", dlc.Title)

			for _, download := range dlc.ParsedDownloads {

				// Check if the language of the download matches the selected language
				if !strings.EqualFold(download.Language, gameLanguage) {
					log.Info().Msgf("Skipping download for language %s because it doesn't match selected language %s", download.Language, gameLanguage)
					fmt.Printf("Skipping downloading game files for %s\n", download.Language)
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
					for _, file := range platformFiles.files {
						if platformName != "all" && platformName != platformFiles.subDir {
							log.Info().Msgf("Skipping DLC download for platform %s because it doesn't match selected platform %s", platformFiles.subDir, platformName)
							fmt.Printf("Skipping downloading DLC files for %s\n", platformFiles.subDir)
							continue
						}

						if file.ManualURL == nil {
							continue
						}

						url := fmt.Sprintf("https://embed.gog.com%s", *file.ManualURL)
						fileName := filepath.Base(*file.ManualURL)
						subDir := filepath.Join("dlcs", platformFiles.subDir)
						taskChan <- downloadTask{url, fileName, subDir,
							resumeFlag, flattenFlag}
					}
				}
			}

			for _, extra := range dlc.Extras {
				if !extrasFlag {
					log.Info().Msgf("Skipping extras for the DLC %s: %s", dlc.Title, extra.Name)
					fmt.Printf("Skipping downloading extras for the DLC %s\n", dlc.Title)
					continue
				}

				extraURL := fmt.Sprintf("https://embed.gog.com%s", extra.ManualURL)
				extraFileName := SanitizePath(extra.Name)
				subDir := filepath.Join("dlcs", "extras")
				taskChan <- downloadTask{extraURL, extraFileName,
					subDir, resumeFlag, flattenFlag}
			}
		}
	}

	close(taskChan)
	wg.Wait()

	metadata, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode game metadata")
		return err
	}
	metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
	if err := os.WriteFile(metadataPath, metadata, 0644); err != nil {
		log.Error().Err(err).Msgf("Failed to save game metadata to %s", metadataPath)
		return err
	}

	return nil
}
