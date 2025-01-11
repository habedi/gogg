package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/chromedp/chromedp"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// AuthGOG logs in to GOG.com and retrieves an access token for the user.
func AuthGOG(authURL string, user *db.User, headless bool) error {

	if user == nil {
		return fmt.Errorf("user data is empy. Please run 'gogg init' to initialize the database")
	}

	if isTokenValid(user) {
		log.Info().Msg("Access token is still valid")
		return nil
	}

	if user.RefreshToken != "" {
		log.Info().Msg("Refreshing access token")
		return refreshToken(user)
	}

	ctx, cancel := createChromeContext(headless)
	defer cancel()

	finalURL, err := performLogin(ctx, authURL, user)
	if err != nil {
		return fmt.Errorf("failed during automated login: %w", err)
	}

	code, err := extractAuthCode(finalURL)
	if err != nil {
		return err
	}

	token, refreshToken, expiresAt, err := exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	user.AccessToken = token
	user.RefreshToken = refreshToken
	user.ExpiresAt = expiresAt
	return db.UpsertUserData(user)
}

// SaveUserCredentials saves the user's credentials in the database.
func SaveUserCredentials(username, password string) error {
	user := &db.User{
		Username:     username,
		Password:     password,
		AccessToken:  "",
		RefreshToken: "",
		ExpiresAt:    "",
	}
	return db.UpsertUserData(user)
}

// refreshToken refreshes the access token using the refresh token.
func refreshToken(user *db.User) error {
	tokenURL := "https://auth.gog.com/token"
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
		"grant_type":    {"refresh_token"},
		"refresh_token": {user.RefreshToken},
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

	user.AccessToken = result.AccessToken
	user.RefreshToken = result.RefreshToken
	user.ExpiresAt = time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	return db.UpsertUserData(user)
}

// isTokenValid checks if the access token (stored in the database) is still valid.
func isTokenValid(user *db.User) bool {

	if user == nil {
		return false
	}

	if user.AccessToken == "" || user.ExpiresAt == "" {
		return false
	}

	expiresAt, err := time.Parse(time.RFC3339, user.ExpiresAt)
	if err != nil {
		log.Error().Err(err).Msg("Invalid expiration time format")
		return false
	}

	return time.Now().Before(expiresAt)
}

// createChromeContext creates a new ChromeDP context.
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

// performLogin performs the login process using ChromeDP.
func performLogin(ctx context.Context, authURL string, user *db.User) (string, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()

	var finalURL string
	err := chromedp.Run(timeoutCtx,
		chromedp.Navigate(authURL),
		chromedp.WaitVisible(`#login_username`, chromedp.ByID),
		chromedp.SendKeys(`#login_username`, user.Username, chromedp.ByID),
		chromedp.SendKeys(`#login_password`, user.Password, chromedp.ByID),
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

// extractAuthCode extracts the authorization code from the URL.
func extractAuthCode(authURL string) (string, error) {
	parsedURL, err := url.Parse(authURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	code := parsedURL.Query().Get("code")
	if code == "" {
		return "", errors.New("authorization code not found in URL")
	}

	return code, nil
}

// exchangeCodeForToken exchanges the authorization code for an access token and a refresh token.
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
