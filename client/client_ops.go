package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/schollz/progressbar/v3"
)

//
// === Data Types & JSON Parsing ===
//

// Game contains information about a game and its downloadable content.
type Game struct {
	Title           string         `json:"title"`
	BackgroundImage *string        `json:"backgroundImage,omitempty"`
	Downloads       []Downloadable `json:"downloads"`
	Extras          []Extra        `json:"extras"`
	DLCs            []DLC          `json:"dlcs"`
}

// PlatformFile contains information about a platform-specific installation file.
type PlatformFile struct {
	ManualURL *string `json:"manualUrl,omitempty"`
	Name      string  `json:"name"`
	Version   *string `json:"version,omitempty"`
	Date      *string `json:"date,omitempty"`
	Size      string  `json:"size"`
}

// Extra contains information about an extra file like a manual or soundtrack.
type Extra struct {
	Name      string `json:"name"`
	Size      string `json:"size"`
	ManualURL string `json:"manualUrl"`
}

// DLC contains information about downloadable content like expansions.
type DLC struct {
	Title           string          `json:"title"`
	BackgroundImage *string         `json:"backgroundImage,omitempty"`
	Downloads       [][]interface{} `json:"downloads"`
	Extras          []Extra         `json:"extras"`
	ParsedDownloads []Downloadable  `json:"-"`
}

// Platform contains information about platform-specific installation files.
type Platform struct {
	Windows []PlatformFile `json:"windows,omitempty"`
	Mac     []PlatformFile `json:"mac,omitempty"`
	Linux   []PlatformFile `json:"linux,omitempty"`
}

// Downloadable contains information about a downloadable file for a specific language and platform.
type Downloadable struct {
	Language  string   `json:"language"`
	Platforms Platform `json:"platforms"`
}

// UnmarshalJSON implements custom unmarshalling for Game to process downloads and DLCs.
func (gd *Game) UnmarshalJSON(data []byte) error {
	type Alias Game
	aux := &struct {
		RawDownloads [][]interface{} `json:"downloads"`
		*Alias
	}{
		Alias: (*Alias)(gd),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Process downloads for the game.
	gd.Downloads = parseRawDownloads(aux.RawDownloads)

	// Process downloads for each DLC.
	for i, dlc := range gd.DLCs {
		gd.DLCs[i].ParsedDownloads = parseRawDownloads(dlc.Downloads)
	}
	return nil
}

// parseRawDownloads converts raw downloads data into a slice of Downloadable.
func parseRawDownloads(rawDownloads [][]interface{}) []Downloadable {
	var downloads []Downloadable
	for _, raw := range rawDownloads {
		if len(raw) != 2 {
			continue
		}
		language, ok := raw[0].(string)
		if !ok {
			continue
		}
		platforms, err := parsePlatforms(raw[1])
		if err != nil {
			continue
		}
		downloads = append(downloads, Downloadable{
			Language:  language,
			Platforms: platforms,
		})
	}
	return downloads
}

// parsePlatforms decodes the platforms object.
func parsePlatforms(data interface{}) (Platform, error) {
	platformsData, err := json.Marshal(data)
	if err != nil {
		return Platform{}, err
	}
	var platforms Platform
	if err := json.Unmarshal(platformsData, &platforms); err != nil {
		return Platform{}, err
	}
	return platforms, nil
}

//
// === Public Utility Functions ===
//

// ParseGameData parses raw game data JSON into a Game struct.
func ParseGameData(data string) (Game, error) {
	var rawResponse Game
	if err := json.Unmarshal([]byte(data), &rawResponse); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return Game{}, err
	}
	return rawResponse, nil
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

// ensureDirExists checks for a directory and creates it if needed.
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

//
// === Downloading Game Files ===
//

// downloadTask represents a file download job.
type downloadTask struct {
	url      string
	fileName string
	subDir   string
	resume   bool
	flatten  bool
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
	if resp.StatusCode == http.StatusFound || resp.StatusCode == http.StatusMovedPermanently {
		location := resp.Header.Get("Location")
		if location != "" {
			log.Info().Msgf("Redirected to: %s", location)
			return location, nil
		}
		log.Info().Msg("Redirect location not found")
	} else {
		log.Info().Msgf("Response status: %d", resp.StatusCode)
	}
	return "", nil
}

// processDownloadTask handles a single download task.
func processDownloadTask(task downloadTask, accessToken, downloadPath, gameTitle string, client, clientNoRedirect *http.Client) error {
	urlStr := task.url
	fileName := task.fileName
	subDir := task.subDir
	resume := task.resume
	flatten := task.flatten

	// Check for redirect location.
	if redirect, err := findRedirectLocation(accessToken, urlStr, clientNoRedirect); err == nil && redirect != "" {
		urlStr = redirect
		fileName = filepath.Base(redirect)
	}

	decodedFileName, err := url.QueryUnescape(fileName)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to decode file name %s", fileName)
		return err
	}
	fileName = decodedFileName

	if flatten {
		subDir = ""
	} else {
		subDir = SanitizePath(subDir)
	}

	gameDir := SanitizePath(gameTitle)
	filePath := filepath.Join(downloadPath, gameDir, subDir, fileName)
	if err := ensureDirExists(filepath.Dir(filePath)); err != nil {
		log.Error().Err(err).Msgf("Failed to prepare directory for %s", filePath)
		return err
	}

	// Handle file creation/resume.
	var file *os.File
	var startOffset int64
	if resume {
		if fileInfo, err := os.Stat(filePath); err == nil {
			startOffset = fileInfo.Size()
			log.Info().Msgf("Resuming download for %s from offset %d", fileName, startOffset)
			file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0o644)
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

	// Use HEAD to get total file size.
	req, err := http.NewRequest("HEAD", urlStr, nil)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create HEAD request for %s", fileName)
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to get file info for %s", fileName)
		return err
	}
	resp.Body.Close()
	totalSize := resp.ContentLength

	// Handle case where Content-Length is not provided (ContentLength = -1)
	if totalSize < 0 {
		log.Warn().Msgf("Content-Length not provided for %s, cannot determine total size", fileName)
		totalSize = 0 // Set to 0 to indicate unknown size
	}

	if totalSize > 0 && startOffset >= totalSize {
		log.Info().Msgf("File %s is already fully downloaded (%d bytes). Skipping.", fileName, totalSize)
		fmt.Printf("File %s is already fully downloaded. Skipping download.\n", fileName)
		return nil
	}

	// Prepare GET request (with Range header if resuming).
	req, err = http.NewRequest("GET", urlStr, nil)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create GET request for %s", fileName)
		return err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	if resume && startOffset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
	}

	resp, err = client.Do(req)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to download %s", fileName)
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("failed to download %s: HTTP %d", fileName, resp.StatusCode)
	}

	// Setup progress bar.
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
	if err := progressBar.Set64(startOffset); err != nil {
		return fmt.Errorf("failed to set progress bar offset: %w", err)
	}
	progressReader := io.TeeReader(resp.Body, progressBar)
	if _, err := io.Copy(file, progressReader); err != nil {
		return fmt.Errorf("failed to save file %s: %w", filePath, err)
	}
	return nil
}

// DownloadGameFiles downloads game files, extras, and DLCs to the given path.
func DownloadGameFiles(accessToken string, game Game, downloadPath string,
	gameLanguage string, platformName string, extrasFlag bool, dlcFlag bool, resumeFlag bool,
	flattenFlag bool, numThreads int,
) error {
	// HTTP clients.
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

	tasks := make(chan downloadTask, 10)
	var wg sync.WaitGroup

	// Start worker goroutines.
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				if err := processDownloadTask(task, accessToken, downloadPath, game.Title, client, clientNoRedirect); err != nil {
					log.Error().Err(err).Msgf("Failed to download %s", task.fileName)
				}
			}
		}()
	}

	// Enqueue tasks for game downloads.
	for _, download := range game.Downloads {
		if !strings.EqualFold(download.Language, gameLanguage) {
			log.Info().Msgf("Skipping download for language %s (selected: %s)", download.Language, gameLanguage)
			fmt.Printf("Skipping game files for %s\n", download.Language)
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
			for _, file := range group.files {
				if platformName != "all" && platformName != group.subDir {
					log.Info().Msgf("Skipping platform %s (selected: %s)", group.subDir, platformName)
					fmt.Printf("Skipping game files for %s\n", group.subDir)
					continue
				}
				if file.ManualURL == nil {
					continue
				}
				rawURL := *file.ManualURL
				if !strings.HasPrefix(rawURL, "http") {
					rawURL = fmt.Sprintf("https://embed.gog.com%s", rawURL)
				}
				tasks <- downloadTask{
					url:      rawURL,
					fileName: filepath.Base(rawURL),
					subDir:   group.subDir,
					resume:   resumeFlag,
					flatten:  flattenFlag,
				}
			}
		}
	}

	// Enqueue tasks for extras.
	for _, extra := range game.Extras {
		if !extrasFlag {
			log.Info().Msgf("Skipping extra %s", extra.Name)
			fmt.Printf("Skipping extras for %s\n", extra.Name)
			continue
		}
		rawURL := extra.ManualURL
		if !strings.HasPrefix(rawURL, "http") {
			rawURL = fmt.Sprintf("https://embed.gog.com%s", rawURL)
		}
		tasks <- downloadTask{
			url:      rawURL,
			fileName: filepath.Base(rawURL),
			subDir:   "extras",
			resume:   resumeFlag,
			flatten:  flattenFlag,
		}
	}

	// Enqueue tasks for DLCs.
	if dlcFlag {
		for _, dlc := range game.DLCs {
			log.Info().Msgf("Processing DLC: %s", dlc.Title)
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, gameLanguage) {
					log.Info().Msgf("Skipping DLC download for language %s (selected: %s)", download.Language, gameLanguage)
					fmt.Printf("Skipping DLC files for %s\n", download.Language)
					continue
				}
				for _, group := range []struct {
					files  []PlatformFile
					subDir string
				}{
					{download.Platforms.Windows, "windows"},
					{download.Platforms.Mac, "mac"},
					{download.Platforms.Linux, "linux"},
				} {
					for _, file := range group.files {
						if platformName != "all" && platformName != group.subDir {
							log.Info().Msgf("Skipping DLC platform %s (selected: %s)", group.subDir, platformName)
							fmt.Printf("Skipping DLC files for %s\n", group.subDir)
							continue
						}
						if file.ManualURL == nil {
							continue
						}
						rawURL := *file.ManualURL
						if !strings.HasPrefix(rawURL, "http") {
							rawURL = fmt.Sprintf("https://embed.gog.com%s", rawURL)
						}
						subDir := filepath.Join("dlcs", group.subDir)
						tasks <- downloadTask{
							url:      rawURL,
							fileName: filepath.Base(rawURL),
							subDir:   subDir,
							resume:   resumeFlag,
							flatten:  flattenFlag,
						}
					}
				}
			}
			for _, extra := range dlc.Extras {
				if !extrasFlag {
					log.Info().Msgf("Skipping DLC extra for %s: %s", dlc.Title, extra.Name)
					fmt.Printf("Skipping DLC extras for %s\n", dlc.Title)
					continue
				}
				rawURL := extra.ManualURL
				if !strings.HasPrefix(rawURL, "http") {
					rawURL = fmt.Sprintf("https://embed.gog.com%s", rawURL)
				}
				subDir := filepath.Join("dlcs", "extras")
				tasks <- downloadTask{
					url:      rawURL,
					fileName: filepath.Base(rawURL),
					subDir:   subDir,
					resume:   resumeFlag,
					flatten:  flattenFlag,
				}
			}
		}
	}

	close(tasks)
	wg.Wait()

	// Save game metadata.
	metadata, err := json.MarshalIndent(game, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode game metadata")
		return err
	}
	metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
	if err := os.WriteFile(metadataPath, metadata, 0o644); err != nil {
		log.Error().Err(err).Msgf("Failed to save game metadata to %s", metadataPath)
		return err
	}

	return nil
}

//
// === HTTP Helper Functions ===
//

// createRequest creates an HTTP request with the specified method, URL, and access token.
func createRequest(method, urlStr, accessToken string) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	return req, nil
}

// sendRequest sends an HTTP request and returns the response.
func sendRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Msg("Failed to send request")
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Error().Msgf("HTTP request failed with status %d", resp.StatusCode)
		return nil, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}
	return resp, nil
}

// readResponseBody reads and returns the response body.
func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		return nil, err
	}
	return body, nil
}

// parseGameData decodes the response body into the provided Game struct.
func parseGameData(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return err
	}
	return nil
}

// parseOwnedGames parses the list of owned game IDs from the response.
func parseOwnedGames(body []byte) ([]int, error) {
	var response struct {
		Owned []int `json:"owned"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.Error().Err(err).Msg("Failed to parse response")
		return nil, err
	}
	return response.Owned, nil
}

//
// === Game Data & Ownership Fetching ===
//

// FetchGameData retrieves game data from GOG.
func FetchGameData(accessToken string, urlStr string) (Game, string, error) {
	req, err := createRequest("GET", urlStr, accessToken)
	if err != nil {
		return Game{}, "", err
	}
	resp, err := sendRequest(req)
	if err != nil {
		return Game{}, "", err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return Game{}, "", err
	}

	var game Game
	if err := parseGameData(body, &game); err != nil {
		return Game{}, "", err
	}
	return game, string(body), nil
}

// FetchIdOfOwnedGames retrieves owned game IDs from GOG.
func FetchIdOfOwnedGames(accessToken string, apiURL string) ([]int, error) {
	req, err := createRequest("GET", apiURL, accessToken)
	if err != nil {
		return nil, err
	}
	resp, err := sendRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}
	return parseOwnedGames(body)
}

//
// === Login & Token Refresh ===
//

// GOGLoginURL is the URL used to log in to GOG.com.
var GOGLoginURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
	"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
	"&response_type=code&layout=client2"

// Login performs login to GOG.com and saves the token record.
func Login(loginURL string, username string, password string, headless bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}

	ctx, cancel := createChromeContext(headless)
	defer cancel()

	log.Info().Msg("Trying to login to GOG.com.")
	finalURL, err := performLogin(ctx, loginURL, username, password, headless)
	if err != nil {
		if headless {
			log.Warn().Err(err).Msg("Headless login failed, retrying with window mode.")
			fmt.Println("Headless login failed, retrying with window mode.")
			ctx, cancel = createChromeContext(false)
			defer cancel()
			finalURL, err = performLogin(ctx, loginURL, username, password, false)
			if err != nil {
				return fmt.Errorf("failed to login: %w", err)
			}
		} else {
			return fmt.Errorf("failed to login: %w", err)
		}
	}

	code, err := extractAuthCode(finalURL)
	if err != nil {
		return err
	}

	token, refreshToken, expiresAt, err := exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	log.Info().Msgf("Access token: %s", token[:10])
	log.Info().Msgf("Refresh token: %s", refreshToken[:10])
	log.Info().Msgf("Expires at: %s", expiresAt)

	return db.UpsertTokenRecord(&db.Token{AccessToken: token, RefreshToken: refreshToken, ExpiresAt: expiresAt})
}

// RefreshToken checks if the token is valid and refreshes it if needed.
func RefreshToken() (*db.Token, error) {
	token, err := db.GetTokenRecord()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token record: %w", err)
	}

	valid, err := isTokenValid(token)
	if err != nil {
		return nil, fmt.Errorf("failed to check token validity: %w", err)
	} else if !valid {
		if err := refreshAccessToken(token); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	return token, nil
}

// Default client credentials
const (
	DefaultClientID     = "46899977096215655"
	DefaultClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
)

// getClientCredentials returns the client ID and secret from environment variables
// or falls back to default values if not set.
func getClientCredentials() (string, string) {
	clientID := os.Getenv("GOGG_CLIENT_ID")
	if clientID == "" {
		clientID = DefaultClientID
	}

	clientSecret := os.Getenv("GOGG_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = DefaultClientSecret
	}

	return clientID, clientSecret
}

// refreshAccessToken refreshes the token using the refresh token.
func refreshAccessToken(token *db.Token) error {
	tokenURL := "https://auth.gog.com/token"
	clientID, clientSecret := getClientCredentials()

	query := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {token.RefreshToken},
	}

	resp, err := http.PostForm(tokenURL, query)
	if err != nil {
		return fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	token.AccessToken = result.AccessToken
	token.RefreshToken = result.RefreshToken
	token.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	return db.UpsertTokenRecord(token)
}

// isTokenValid checks if the token stored in the database is still valid.
func isTokenValid(token *db.Token) (bool, error) {
	if token == nil {
		return false, fmt.Errorf("access token data does not exist in the database. Please login first")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresAt == "" {
		return false, nil
	}
	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		log.Error().Msgf("Failed to parse expiration time: %s", token.ExpiresAt)
		return false, err
	}
	return time.Now().Before(expiresAt), nil
}

// createChromeContext creates a ChromeDP context with or without headless mode.
func createChromeContext(headless bool) (context.Context, context.CancelFunc) {
	var execPath string
	if path, err := exec.LookPath("google-chrome"); err == nil {
		execPath = path
	} else if path, err := exec.LookPath("chromium"); err == nil {
		execPath = path
	} else if path, err := exec.LookPath("chrome"); err == nil {
		execPath = path
	} else {
		log.Error().Msg("Neither Google Chrome nor Chromium is available in the path. Please install one of them.")
		return nil, nil
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
	)
	if !headless {
		opts = append(opts, chromedp.Flag("headless", false), chromedp.Flag("disable-gpu", false), chromedp.Flag("start-maximized", true))
	}

	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelContext := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Info().Msgf))
	return ctx, func() {
		cancelContext()
		cancelAllocator()
	}
}

// performLogin runs the login process using ChromeDP and returns the final URL.
func performLogin(ctx context.Context, loginURL string, username string, password string, headlessMode bool) (string, error) {
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	if headlessMode {
		timeoutCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	} else {
		timeoutCtx, cancel = context.WithTimeout(ctx, 4*time.Minute)
	}
	defer cancel()

	var finalURL string
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`#login_username`, chromedp.ByID),
		chromedp.SendKeys(`#login_username`, username, chromedp.ByID),
		chromedp.SendKeys(`#login_password`, password, chromedp.ByID),
		chromedp.Click(`#login_login`, chromedp.ByID),
		chromedp.ActionFunc(func(ctx context.Context) error {
			maxAttempts := 120 // 60 seconds (120 * 500ms)
			for attempt := 0; attempt < maxAttempts; attempt++ {
				var currentURL string
				if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
					return err
				}
				if strings.Contains(currentURL, "on_login_success") && strings.Contains(currentURL, "code=") {
					finalURL = currentURL
					return nil
				}
				// Check for login failure indicators
				if strings.Contains(currentURL, "login_failure") || strings.Contains(currentURL, "error=") {
					return fmt.Errorf("login failed: detected error in URL: %s", currentURL)
				}
				time.Sleep(500 * time.Millisecond)
			}
			return fmt.Errorf("login timed out after waiting for %d seconds", maxAttempts/2)
		}),
	)
	return finalURL, err
}

// extractAuthCode extracts the authorization code from the final URL.
func extractAuthCode(authURL string) (string, error) {
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}
	code := parsedURL.Query().Get("code")
	if code == "" {
		return "", errors.New("authorization code not found in the URL")
	}
	return code, nil
}

// exchangeCodeForToken exchanges an authorization code for tokens.
func exchangeCodeForToken(code string) (string, string, string, error) {
	tokenURL := "https://auth.gog.com/token"
	clientID, clientSecret := getClientCredentials()

	query := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://embed.gog.com/on_login_success?origin=client"},
	}
	resp, err := http.PostForm(tokenURL, query)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read token response: %w", err)
	}
	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", "", fmt.Errorf("failed to parse token response: %w", err)
	}
	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	return result.AccessToken, result.RefreshToken, expiresAt, nil
}
