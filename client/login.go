package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// GOGLoginURL is the URL to login to GOG.com.
var GOGLoginURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
	"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
	"&response_type=code&layout=client2"

// Login logs in to GOG.com and retrieves an access token for the user.
// It takes the login URL, username, password, and a boolean indicating whether to use headless mode.
// It returns an error if the login process fails.
func Login(loginURL string, username string, password string, headless bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}

	// Try headless login first
	ctx, cancel := createChromeContext(headless)
	defer cancel()

	log.Info().Msg("Trying to login to GOG.com.")

	// Perform the login
	finalURL, err := performLogin(ctx, loginURL, username, password, headless)
	if err != nil {
		if headless {
			log.Warn().Err(err).Msg("Headless login failed, retrying with window mode.")
			fmt.Println("Headless login failed, retrying with window mode.")
			// Retry with window mode if headless login fails
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

	// Extract the authorization code from the final URL after successful login
	code, err := extractAuthCode(finalURL)
	if err != nil {
		return err
	}

	// Exchange the authorization code for an access token and a refresh token
	token, refreshToken, expiresAt, err := exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	// Print the access token, refresh token, and expiration if debug mode is enabled

	log.Info().Msgf("Access token: %s", token[:10])
	log.Info().Msgf("Refresh token: %s", refreshToken[:10])
	log.Info().Msgf("Expires at: %s", expiresAt)

	// Save the token record in the database
	return db.UpsertTokenRecord(&db.Token{AccessToken: token, RefreshToken: refreshToken, ExpiresAt: expiresAt})
}

// RefreshToken refreshes the access token if it is expired and returns the updated token record.
// It returns a pointer to the updated token record and an error if the refresh process fails.
func RefreshToken() (*db.Token, error) {
	token, err := db.GetTokenRecord()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve token record: %w", err)
	}

	// Check if the token is valid and refresh it if necessary
	tokenStatus, err := isTokenValid(token)
	if err != nil {
		return nil, fmt.Errorf("failed to check token validity: %w", err)
	} else if !tokenStatus {
		if err := refreshAccessToken(token); err != nil {
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
	}

	return token, nil
}

// refreshAccessToken refreshes the access token using the refresh token.
// It takes a pointer to the token record and returns an error if the refresh process fails.
func refreshAccessToken(token *db.Token) error {
	tokenURL := "https://auth.gog.com/token"
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
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

// isTokenValid checks if the access token (stored in the database) is still valid.
// It takes a pointer to the token record and returns a boolean indicating whether the token is valid.
func isTokenValid(token *db.Token) (bool, error) {
	if token == nil {
		return false, fmt.Errorf("access token data does not exist in the database. Please login first")
	}

	// Check if the token fields are empty (needs refresh)
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresAt == "" {
		return false, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		log.Error().Msgf("Failed to parse expiration time: %s", token.ExpiresAt)
		return false, err
	} else {
		return time.Now().Before(expiresAt), nil
	}
}

// createChromeContext creates a new ChromeDP context with the specified option to run Chrome in headless mode or not.
// It returns the ChromeDP context and a cancel function to release resources.
func createChromeContext(headless bool) (context.Context, context.CancelFunc) {
	// Check if Google Chrome or Chromium is available in the path
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
		opts = append(opts, chromedp.Flag("headless", false), chromedp.Flag("disable-gpu", false),
			chromedp.Flag("start-maximized", true))
	}

	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelContext := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Info().Msgf))

	return ctx, func() {
		cancelContext()
		cancelAllocator()
	}
}

// performLogin performs the login process using the provided username and password and returns the final URL after successful login.
// It takes the ChromeDP context, login URL, username, password, and a boolean indicating whether to use headless mode as parameters, and returns the final URL and an error if the login process fails.
func performLogin(ctx context.Context, loginURL string, username string, password string,
	headlessMode bool,
) (string, error) {
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	var finalURL string

	if headlessMode {
		// Timeout for login in headless mode 30
		timeoutCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	} else {
		// Timeout for login in window mode 4 minutes
		timeoutCtx, cancel = context.WithTimeout(ctx, 4*time.Minute)
	}

	// Close the context when the function returns
	defer cancel()

	// Start the login process
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(loginURL),
		chromedp.WaitVisible(`#login_username`, chromedp.ByID),
		chromedp.SendKeys(`#login_username`, username, chromedp.ByID),
		chromedp.SendKeys(`#login_password`, password, chromedp.ByID),
		chromedp.Click(`#login_login`, chromedp.ByID),
		chromedp.ActionFunc(func(ctx context.Context) error {
			for {
				var currentURL string
				if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
					return err
				}
				if strings.Contains(currentURL, "on_login_success") && strings.Contains(currentURL, "code=") {
					finalURL = currentURL
					return nil
				}
				time.Sleep(500 * time.Millisecond)
			}
		}),
	)
	return finalURL, err
}

// extractAuthCode extracts the authorization code from the URL after successful login.
// It takes the authorization URL as a parameter and returns the authorization code and an error if the extraction fails.
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

// exchangeCodeForToken exchanges the authorization code for an access token and a refresh token.
// It takes the authorization code as a parameter and returns the access token, refresh token, expiration time, and an error if the exchange process fails.
func exchangeCodeForToken(code string) (string, string, string, error) {
	tokenURL := "https://auth.gog.com/token"
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
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
