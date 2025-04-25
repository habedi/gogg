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
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
)

// GOGLoginURL is the URL used to initiate the GOG login flow.
var GOGLoginURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
	"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
	"&response_type=code&layout=client2"

// Default client credentials used for token exchange if not overridden by ENV vars.
const (
	DefaultClientID     = "46899977096215655"
	DefaultClientSecret = "9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"
)

// Login performs login to GOG.com using chromedp for browser automation and saves the token record.
func Login(loginURL string, username string, password string, headless bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}

	// Attempt login, potentially retrying without headless if the first attempt fails.
	attemptLogin := func(useHeadless bool) (string, error) {
		ctx, cancel := createChromeContext(useHeadless)
		if cancel == nil { // createChromeContext returns nil cancel if browser not found
			return "", fmt.Errorf("failed to create browser context (Chrome/Chromium not found?)")
		}
		defer cancel() // Important to close the browser context

		log.Info().Bool("headless", useHeadless).Msg("Attempting GOG login")
		return performLogin(ctx, loginURL, username, password, useHeadless)
	}

	finalURL, err := attemptLogin(headless)
	if err != nil && headless {
		// If headless failed, try again with a visible browser window.
		log.Warn().Err(err).Msg("Headless login failed, retrying with window mode.")
		fmt.Println("Headless login failed, retrying with window mode. Please complete login in the browser.")
		finalURL, err = attemptLogin(false)
	}

	// Handle final error after potential retry.
	if err != nil {
		return fmt.Errorf("failed to login: %w", err)
	}
	log.Info().Msg("Browser login part successful, obtained redirect URL.")

	// Extract authorization code from the final URL.
	code, err := extractAuthCode(finalURL)
	if err != nil {
		return err // Error already contains context
	}
	log.Info().Msg("Extracted authorization code.")

	// Exchange the code for access and refresh tokens.
	accessToken, refreshToken, expiresAtStr, err := exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}
	log.Info().Msg("Successfully exchanged code for tokens.")
	// Log snippets for verification, avoid logging full tokens.
	log.Debug().Str("access_token_prefix", accessToken[:min(len(accessToken), 8)]).
		Str("refresh_token_prefix", refreshToken[:min(len(refreshToken), 8)]).
		Str("expires_at", expiresAtStr).Msg("Token details")

	// Store the obtained tokens in the database.
	tokenRecord := &db.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAtStr,
	}
	if err := db.UpsertTokenRecord(tokenRecord); err != nil {
		return fmt.Errorf("failed to save token record to database: %w", err)
	}
	log.Info().Msg("Token record saved successfully.")

	return nil
}

// RefreshToken retrieves the stored token, checks its validity, refreshes it if necessary,
// and returns the potentially updated token.
func RefreshToken() (*db.Token, error) {
	log.Debug().Msg("Checking token status...")
	token, err := db.GetTokenRecord()
	if err != nil {
		// This specifically includes the case where the record doesn't exist yet.
		return nil, fmt.Errorf("failed to retrieve token record: %w", err)
	}
	if token == nil {
		return nil, errors.New("no token record found in database. Please login first")
	}

	valid, err := isTokenValid(token)
	if err != nil {
		// Error parsing the existing expiry date.
		return nil, fmt.Errorf("failed to check token validity: %w", err)
	}

	if !valid {
		log.Info().Msg("Access token is expired or invalid, attempting refresh.")
		if err := refreshAccessToken(token); err != nil {
			// If refresh fails, return the error. The original token object
			// might still be useful for debugging but shouldn't be used.
			return nil, fmt.Errorf("failed to refresh token: %w", err)
		}
		log.Info().Msg("Token refreshed successfully.")
		// The 'token' object passed to refreshAccessToken should have been updated in place.
	} else {
		log.Debug().Msg("Existing access token is still valid.")
	}

	// Return the token (which might have been updated by refreshAccessToken).
	return token, nil
}

// getClientCredentials returns the client ID and secret, using environment variables
// GOGG_CLIENT_ID and GOGG_CLIENT_SECRET as overrides, otherwise defaults.
func getClientCredentials() (string, string) {
	clientID := os.Getenv("GOGG_CLIENT_ID")
	if clientID == "" {
		clientID = DefaultClientID
		log.Debug().Msg("Using default GOG Client ID")
	} else {
		log.Debug().Msg("Using GOG Client ID from environment variable")
	}

	clientSecret := os.Getenv("GOGG_CLIENT_SECRET")
	if clientSecret == "" {
		clientSecret = DefaultClientSecret
		log.Debug().Msg("Using default GOG Client Secret")
	} else {
		log.Debug().Msg("Using GOG Client Secret from environment variable")
	}

	return clientID, clientSecret
}

// refreshAccessToken attempts to get a new access token using the refresh token.
// It updates the passed *db.Token object in place if successful and saves it.
func refreshAccessToken(token *db.Token) error {
	if token.RefreshToken == "" {
		return errors.New("cannot refresh token: refresh token is missing")
	}

	tokenURL := "https://auth.gog.com/token"
	clientID, clientSecret := getClientCredentials()

	data := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {token.RefreshToken},
	}

	log.Info().Msg("Sending token refresh request")
	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token refresh response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
		log.Error().Err(err).Int("status", resp.StatusCode).Str("body", string(body)).Msg("Token refresh HTTP error")
		return err
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"` // Typically seconds until expiry
		RefreshToken string `json:"refresh_token"`
		UserID       string `json:"user_id"` // GOG includes this, might be useful later
		Scope        string `json:"scope"`   // Granted scopes
		SessionID    string `json:"session_id"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("Failed to parse token refresh response JSON")
		return fmt.Errorf("failed to parse token refresh response: %w", err)
	}

	// Update the token object passed into the function
	token.AccessToken = result.AccessToken
	token.RefreshToken = result.RefreshToken // GOG might issue a new refresh token
	token.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)

	log.Debug().Str("access_token_prefix", token.AccessToken[:min(len(token.AccessToken), 8)]).
		Str("refresh_token_prefix", token.RefreshToken[:min(len(token.RefreshToken), 8)]).
		Str("expires_at", token.ExpiresAt).Msg("Refreshed token details")

	// Save the updated token back to the database
	if err := db.UpsertTokenRecord(token); err != nil {
		return fmt.Errorf("failed to save refreshed token to database: %w", err)
	}

	return nil
}

// isTokenValid checks if the access token seems valid based on its expiry time.
// It does not guarantee the token works, only that it hasn't expired yet.
func isTokenValid(token *db.Token) (bool, error) {
	if token == nil {
		// This case should ideally be handled before calling, but check defensively.
		return false, errors.New("token record is nil")
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.ExpiresAt == "" {
		// If essential parts are missing, treat as invalid.
		log.Warn().Msg("Token record is incomplete (missing access, refresh, or expiry)")
		return false, nil
	}

	expiresAtTime, err := time.Parse(time.RFC3339, token.ExpiresAt)
	if err != nil {
		log.Error().Err(err).Str("expires_at", token.ExpiresAt).Msg("Failed to parse token expiration time")
		// Consider the token invalid if the expiry date can't be parsed.
		return false, fmt.Errorf("invalid expires_at format: %w", err)
	}

	// Check if the current time is before the expiration time.
	// Add a small buffer (e.g., 1 minute) to be safe?
	// valid := time.Now().Add(1 * time.Minute).Before(expiresAtTime)
	valid := time.Now().Before(expiresAtTime)
	log.Debug().Bool("valid", valid).Time("expires_at", expiresAtTime).Msg("Token validity check")
	return valid, nil
}

// createChromeContext sets up the execution context for chromedp.
func createChromeContext(headless bool) (context.Context, context.CancelFunc) {
	// Find Chrome/Chromium executable path
	var execPath string
	var err error
	browserCandidates := []string{"google-chrome", "chromium", "chrome"} // Order matters if multiple are installed
	for _, candidate := range browserCandidates {
		execPath, err = exec.LookPath(candidate)
		if err == nil {
			log.Debug().Str("path", execPath).Msgf("Found browser executable: %s", candidate)
			break
		}
	}
	if execPath == "" {
		log.Error().Strs("candidates", browserCandidates).Msg("Could not find any compatible browser executable in PATH.")
		return nil, nil // Signal failure
	}

	// Configure chromedp options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.Flag("disable-extensions", true), // Good practice for automation
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		// Add other potentially useful flags:
		// chromedp.Flag("disable-gpu", true), // Often needed in headless/server environments
		// chromedp.Flag("disable-software-rasterizer", true),
		// chromedp.Flag("mute-audio", true),
	)

	// Apply headless mode settings specifically
	if headless {
		opts = append(opts, chromedp.Flag("headless", true))
		log.Debug().Msg("Configuring chromedp for headless mode")
	} else {
		// For non-headless, explicitly disable headless and ensure GPU might be needed
		opts = append(opts,
			chromedp.Flag("headless", false),
			chromedp.Flag("disable-gpu", false),     // Might be needed for UI rendering
			chromedp.Flag("start-maximized", false), // Or true if preferred
		)
		log.Debug().Msg("Configuring chromedp for non-headless (window) mode")
	}

	// Create the allocator context
	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create the main browser context, potentially with logging
	// Use WithTimeout if a global timeout for the context is desired,
	// but individual Run calls usually handle their own timeouts.
	ctx, cancelContext := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Printf)) // Log chromedp actions

	// Return a combined cancel function
	combinedCancel := func() {
		cancelContext()
		cancelAllocator()
		log.Debug().Msg("Chromedp context cancelled")
	}

	return ctx, combinedCancel
}

// performLogin uses chromedp to automate the login process within the browser context.
func performLogin(ctx context.Context, loginURL string, username string, password string, headlessMode bool) (string, error) {
	// Set a timeout for the entire login interaction.
	// Be more generous if not headless, as user interaction might be needed (e.g., 2FA, CAPTCHA).
	var interactionTimeout time.Duration
	if headlessMode {
		interactionTimeout = 45 * time.Second // Shorter timeout for automated flow
	} else {
		interactionTimeout = 5 * time.Minute // Longer timeout for potential user interaction
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, interactionTimeout)
	defer cancel()

	var finalURL string
	log.Info().Msg("Navigating to GOG login page and entering credentials...")
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(loginURL),
		// Wait for the username field to be visible before interacting.
		chromedp.WaitVisible(`#login_username`, chromedp.ByID),
		chromedp.SendKeys(`#login_username`, username, chromedp.ByID),
		chromedp.SendKeys(`#login_password`, password, chromedp.ByID),
		// Click the login button.
		chromedp.Click(`#login_button > button`, chromedp.ByQuery), // More specific selector for the button
		// Wait for redirection to the success URL or an error indicator.
		chromedp.ActionFunc(func(ctx context.Context) error {
			log.Info().Msg("Waiting for login result (redirect or error)...")
			// Poll the URL until success, failure, or timeout.
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			startTime := time.Now()
			pollTimeout := 30 * time.Second // Max time to wait for redirect after click

			for {
				select {
				case <-ctx.Done(): // Outer context timed out or was cancelled
					return fmt.Errorf("login context cancelled or timed out: %w", ctx.Err())
				case <-time.After(pollTimeout): // Polling timed out
					return fmt.Errorf("timed out waiting for login redirect after %v", pollTimeout)
				case <-ticker.C:
					var currentURL string
					if err := chromedp.Location(&currentURL).Do(ctx); err != nil {
						log.Warn().Err(err).Msg("Failed to get current URL during login poll")
						continue // Try again on next tick
					}

					// Check for success condition (specific redirect URL pattern).
					if strings.Contains(currentURL, "embed.gog.com/on_login_success") && strings.Contains(currentURL, "code=") {
						log.Info().Msg("Login successful: Detected success URL.")
						finalURL = currentURL
						return nil // Success
					}

					// Check for known failure conditions (URL patterns, visible error messages).
					if strings.Contains(currentURL, "login/error") || strings.Contains(currentURL, "login_failure") || strings.Contains(currentURL, "error=") {
						log.Error().Str("url", currentURL).Msg("Login failed: Detected error in URL.")
						return fmt.Errorf("login failed: error indicated in URL: %s", currentURL)
					}

					// Optional: Check for visible error messages on the page
					// var errorVisible bool
					// err := chromedp.Run(ctx, chromedp.EvaluateAsDevTools(`document.querySelector('.error-message-selector') !== null`, &errorVisible))
					// if err == nil && errorVisible {
					//    log.Error().Msg("Login failed: Detected visible error message on page.")
					//    return errors.New("login failed: detected error message on page")
					// }

					log.Debug().Dur("elapsed", time.Since(startTime)).Str("current_url", currentURL).Msg("Polling login status...")
				}
			}
		}),
	)
	if err != nil {
		// Provide more context about the failure if possible
		log.Error().Err(err).Msg("Chromedp run for login failed")
		return "", fmt.Errorf("login automation failed: %w", err)
	}

	if finalURL == "" {
		// Should not happen if ActionFunc returns nil, but check defensively.
		return "", errors.New("login automation finished without error, but failed to capture final URL")
	}

	return finalURL, nil
}

// extractAuthCode parses the final redirect URL to get the authorization code.
func extractAuthCode(authURL string) (string, error) {
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		log.Error().Err(err).Str("url", authURL).Msg("Failed to parse final redirect URL")
		return "", fmt.Errorf("failed to parse authorization URL: %w", err)
	}

	code := parsedURL.Query().Get("code")
	if code == "" {
		log.Error().Str("url", authURL).Msg("Authorization code not found in URL query parameters")
		return "", errors.New("authorization code not found in the final URL")
	}

	log.Debug().Str("code_prefix", code[:min(len(code), 8)]).Msg("Extracted authorization code")
	return code, nil
}

// exchangeCodeForToken exchanges the authorization code for access and refresh tokens.
func exchangeCodeForToken(code string) (string, string, string, error) {
	tokenURL := "https://auth.gog.com/token"
	clientID, clientSecret := getClientCredentials()
	redirectURI := "https://embed.gog.com/on_login_success?origin=client" // Must match the initial request

	data := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
	}

	log.Info().Msg("Exchanging authorization code for tokens")
	resp, err := http.PostForm(tokenURL, data)
	if err != nil {
		return "", "", "", fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read token exchange response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
		log.Error().Err(err).Int("status", resp.StatusCode).Str("body", string(body)).Msg("Token exchange HTTP error")
		return "", "", "", err
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"` // Seconds
		RefreshToken string `json:"refresh_token"`
		// Include other fields if needed later
	}

	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Err(err).Str("body", string(body)).Msg("Failed to parse token exchange response JSON")
		return "", "", "", fmt.Errorf("failed to parse token exchange response: %w", err)
	}

	if result.AccessToken == "" || result.RefreshToken == "" {
		log.Error().Str("body", string(body)).Msg("Token exchange response missing access or refresh token")
		return "", "", "", errors.New("token exchange response did not contain required tokens")
	}

	// Calculate expiry time
	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	expiresAtStr := expiresAt.Format(time.RFC3339)

	return result.AccessToken, result.RefreshToken, expiresAtStr, nil
}
