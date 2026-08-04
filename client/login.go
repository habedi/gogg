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

var GOGLoginURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
	"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
	"&response_type=code&layout=client2"

type GogClient struct {
	TokenURL string
}

// PerformTokenRefresh performs a token refresh without explicit cancellation support.
// Deprecated: prefer PerformTokenRefreshCtx for new code.
func (c *GogClient) PerformTokenRefresh(refreshToken string) (accessToken string, newRefreshToken string, expiresIn int64, err error) {
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.PostForm(c.TokenURL, query)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to post form for token refresh: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to read token refresh response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", "", 0, fmt.Errorf("token refresh failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error_description"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", 0, fmt.Errorf("failed to parse token refresh response: %w", err)
	}

	if result.Error != "" {
		return "", "", 0, fmt.Errorf("token refresh API error: %s", result.Error)
	}

	return result.AccessToken, result.RefreshToken, result.ExpiresIn, nil
}

func (c *GogClient) Login(loginURL string, username string, password string, headless bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}

	execPath, err := findBrowser()
	if err != nil {
		return err
	}

	ctx, cancel := createChromeContext(execPath, headless)
	defer cancel()

	log.Info().Msg("Trying to login to GOG.com.")

	finalURL, err := performLogin(ctx, loginURL, username, password, headless)
	if err != nil {
		if headless {
			log.Warn().Err(err).Msg("Headless login failed, retrying with window mode.")
			fmt.Println("Headless login failed, retrying with window mode.")

			// Cancel the first headless context before creating a new one.
			cancel()

			headedCtx, headedCancel := createChromeContext(execPath, false)
			defer headedCancel() // Defer cancellation of the new headed context.

			finalURL, err = performLogin(headedCtx, loginURL, username, password, false)
			if err != nil {
				return fmt.Errorf("failed to login using browser %s: %w", execPath, err)
			}
		} else {
			return fmt.Errorf("failed to login using browser %s: %w", execPath, err)
		}
	}

	code, err := extractAuthCode(finalURL)
	if err != nil {
		return err
	}

	return c.exchangeAndStore(code)
}

// LoginWithCode signs in with the authorization code GOG issues after a
// successful login, so no browser has to be driven: the user logs in wherever
// they like and hands the code back. codeOrURL may be the address the browser
// was redirected to or just the code out of it.
func (c *GogClient) LoginWithCode(codeOrURL string) error {
	code, err := parseAuthCode(codeOrURL)
	if err != nil {
		return err
	}
	return c.exchangeAndStore(code)
}

// exchangeAndStore trades an authorization code for tokens and saves them.
func (c *GogClient) exchangeAndStore(code string) error {
	token, refreshToken, expiresAt, err := c.exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("failed to exchange authorization code for token: %w", err)
	}

	log.Info().Str("expires_at", expiresAt).Msg("Received access and refresh tokens")

	return db.UpsertTokenRecord(&db.Token{AccessToken: token, RefreshToken: refreshToken, ExpiresAt: expiresAt})
}

// parseAuthCode accepts the address GOG redirects to after a successful login,
// the query part of that address, or the bare authorization code.
func parseAuthCode(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("no authorization code given")
	}

	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return extractAuthCode(input)
	}

	if strings.Contains(input, "code=") {
		values, err := url.ParseQuery(input)
		if err != nil {
			return "", fmt.Errorf("failed to read the authorization code: %w", err)
		}
		if code := values.Get("code"); code != "" {
			return code, nil
		}
		return "", errors.New("authorization code not found in the pasted address")
	}

	if strings.ContainsAny(input, " \t\r\n/?&=") {
		return "", errors.New("that is neither an authorization code nor the address containing one")
	}
	return input, nil
}

// browserExecutables lists the Chrome-family browsers to look for, in order of
// preference. The same browser goes by different names per platform and
// distribution.
var browserExecutables = []string{
	"google-chrome", "google-chrome-stable", "Google Chrome",
	"chromium", "chromium-browser", "Chromium",
	"chrome",
	"msedge", "microsoft-edge", "microsoft-edge-stable", "Microsoft Edge",
}

// findBrowser returns the browser to drive. GOGG_BROWSER overrides the search,
// which is how to point gogg at a browser that is not on PATH, or away from one
// that cannot be driven, such as a snap-confined Chromium. It accepts a bare
// name or a path.
func findBrowser() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("GOGG_BROWSER")); custom != "" {
		path, err := exec.LookPath(custom)
		if err != nil {
			return "", fmt.Errorf("GOGG_BROWSER is set to %q, which is not an executable: %w", custom, err)
		}
		return path, nil
	}

	for _, browser := range browserExecutables {
		if path, err := exec.LookPath(browser); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no Chrome, Chromium, or Edge executable found in PATH; " +
		"set GOGG_BROWSER to the browser to use, or log in with 'gogg login --code'")
}

func createChromeContext(execPath string, headless bool) (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(execPath),
		chromedp.Flag("headless", headless),
	)

	if headless {
		opts = append(opts, chromedp.Flag("disable-gpu", true))
	}

	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelContext := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Info().Msgf))

	return ctx, func() {
		cancelContext()
		cancelAllocator()
	}
}

func performLogin(ctx context.Context, loginURL string, username string, password string,
	headlessMode bool,
) (string, error) {
	var timeoutCtx context.Context
	var cancel context.CancelFunc
	var finalURL string

	if headlessMode {
		timeoutCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
	} else {
		timeoutCtx, cancel = context.WithTimeout(ctx, 4*time.Minute)
	}
	defer cancel()

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

func (c *GogClient) exchangeCodeForToken(code string) (string, string, string, error) {
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {"https://embed.gog.com/on_login_success?origin=client"},
	}

	resp, err := http.PostForm(c.TokenURL, query)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to exchange code for token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", "", "", fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if result.AccessToken == "" {
		return "", "", "", fmt.Errorf("token response did not contain an access token")
	}

	expiresAt := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second).Format(time.RFC3339)
	return result.AccessToken, result.RefreshToken, expiresAt, nil
}
