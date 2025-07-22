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

var GOGLoginURL = "https://auth.gog.com/auth?client_id=46899977096215655" +
	"&redirect_uri=https%3A%2F%2Fembed.gog.com%2Fon_login_success%3Forigin%3Dclient" +
	"&response_type=code&layout=client2"

// GogClient implements the auth.TokenRefresher interface.
type GogClient struct{}

// PerformTokenRefresh sends a request to GOG's token endpoint to get a new access token.
func (c *GogClient) PerformTokenRefresh(refreshToken string) (accessToken string, newRefreshToken string, expiresIn int64, err error) {
	tokenURL := "https://auth.gog.com/token"
	query := url.Values{
		"client_id":     {"46899977096215655"},
		"client_secret": {"9d85c43b1482497dbbce61f6e4aa173a433796eeae2ca8c5f6129f2dc4de46d9"},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.PostForm(tokenURL, query)
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

func Login(loginURL string, username string, password string, headless bool) error {
	if username == "" || password == "" {
		return fmt.Errorf("username and password cannot be empty")
	}
	ctx, cancel, err := createChromeContext(headless)
	if err != nil {
		return err
	}
	defer cancel()
	log.Info().Msg("Trying to login to GOG.com.")
	finalURL, err := performLogin(ctx, loginURL, username, password, headless)
	if err != nil {
		if headless {
			log.Warn().Err(err).Msg("Headless login failed, retrying with window mode.")
			fmt.Println("Headless login failed, retrying with window mode.")
			ctx, cancel, err = createChromeContext(false)
			if err != nil {
				return fmt.Errorf("failed to create Chrome context: %w", err)
			}
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

func createChromeContext(headless bool) (context.Context, context.CancelFunc, error) {
	var execPath string
	if p, err := exec.LookPath("google-chrome"); err == nil {
		execPath = p
	} else if p, err := exec.LookPath("chromium"); err == nil {
		execPath = p
	} else if p, err := exec.LookPath("chrome"); err == nil {
		execPath = p
	} else {
		return nil, nil, fmt.Errorf("no Chrome or Chromium executable found in PATH")
	}
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(execPath))
	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelContext := chromedp.NewContext(allocatorCtx, chromedp.WithLogf(log.Info().Msgf))
	return ctx, func() {
		cancelContext()
		cancelAllocator()
	}, nil
}

func performLogin(ctx context.Context, loginURL string, username string, password string, headlessMode bool) (string, error) {
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
