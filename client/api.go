package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// --- HTTP Helper Functions (kept private) ---

// createRequest creates an HTTP request with authorization.
func createRequest(method, urlStr, accessToken string) (*http.Request, error) {
	req, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		log.Error().Err(err).Str("method", method).Str("url", urlStr).Msg("Failed to create HTTP request object")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	// Add other common headers if needed, e.g., User-Agent
	// req.Header.Set("User-Agent", "gogg-client/...")
	return req, nil
}

// sendRequest sends an HTTP request using a default client and checks status.
func sendRequest(req *http.Request) (*http.Response, error) {
	// Consider making the client configurable or shared
	client := &http.Client{Timeout: 30 * time.Second}
	log.Debug().Str("method", req.Method).Str("url", req.URL.String()).Msg("Sending HTTP request")
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("method", req.Method).Str("url", req.URL.String()).Msg("HTTP request failed")
		return nil, err
	}
	// Check for non-OK status codes
	// Note: Download logic handles redirects and specific download statuses (200, 206) separately.
	// This check is more for general API calls expecting 200 OK.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Attempt to read body for more error details, but don't fail if body reading fails
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := ""
		if readErr == nil {
			bodyStr = string(bodyBytes)
		}
		resp.Body.Close() // Close body after reading or if status is bad
		err = fmt.Errorf("unexpected HTTP status: %d %s. Body: %s", resp.StatusCode, http.StatusText(resp.StatusCode), bodyStr)
		log.Error().Str("method", req.Method).Str("url", req.URL.String()).Int("status", resp.StatusCode).Str("body", bodyStr).Msg("HTTP request returned non-OK status")
		return nil, err
	}
	log.Debug().Str("method", req.Method).Str("url", req.URL.String()).Int("status", resp.StatusCode).Msg("HTTP request successful")
	return resp, nil
}

// readResponseBody reads and closes the response body.
func readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Str("url", resp.Request.URL.String()).Msg("Failed to read response body")
		return nil, err
	}
	return body, nil
}

// parseGameData specific helper used by FetchGameData
func parseGameDataInternal(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		// Log the problematic body content (or part of it) for debugging if log level allows
		log.Error().Err(err).Str("body_preview", string(body[:min(len(body), 200)])).Msg("Failed to parse game data JSON")
		return err
	}
	return nil
}

// parseOwnedGames specific helper used by FetchIdOfOwnedGames
func parseOwnedGames(body []byte) ([]int, error) {
	var response struct {
		Owned []int `json:"owned"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		log.Error().Err(err).Str("body_preview", string(body[:min(len(body), 200)])).Msg("Failed to parse owned games JSON")
		return nil, err
	}
	return response.Owned, nil
}

// --- GOG API Interaction Functions ---

// FetchGameData retrieves and parses game details data from GOG API.
// It returns the parsed Game struct, the raw JSON string, and any error.
func FetchGameData(accessToken string, urlStr string) (Game, string, error) {
	log.Info().Str("url", urlStr).Msg("Fetching game data")
	req, err := createRequest("GET", urlStr, accessToken)
	if err != nil {
		return Game{}, "", fmt.Errorf("failed to create request for game data: %w", err)
	}

	resp, err := sendRequest(req)
	if err != nil {
		return Game{}, "", fmt.Errorf("failed to send request for game data: %w", err)
	}
	// Note: sendRequest already checks for status 2xx, so we expect OK here.

	body, err := readResponseBody(resp) // Reads and closes body
	if err != nil {
		return Game{}, "", fmt.Errorf("failed to read game data response body: %w", err)
	}

	var game Game
	if err := parseGameDataInternal(body, &game); err != nil {
		// Attempt to return raw body even if parsing fails, might be useful
		return Game{}, string(body), fmt.Errorf("failed to parse game data JSON: %w", err)
	}

	log.Info().Str("game", game.Title).Msg("Successfully fetched and parsed game data")
	return game, string(body), nil
}

// FetchIdOfOwnedGames retrieves owned game IDs from the GOG API.
func FetchIdOfOwnedGames(accessToken string, apiURL string) ([]int, error) {
	log.Info().Str("url", apiURL).Msg("Fetching owned game IDs")
	req, err := createRequest("GET", apiURL, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for owned games: %w", err)
	}

	resp, err := sendRequest(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request for owned games: %w", err)
	}
	// Note: sendRequest already checks for status 2xx.

	body, err := readResponseBody(resp) // Reads and closes body
	if err != nil {
		return nil, fmt.Errorf("failed to read owned games response body: %w", err)
	}

	ids, err := parseOwnedGames(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse owned games JSON: %w", err)
	}

	log.Info().Int("count", len(ids)).Msg("Successfully fetched owned game IDs")
	return ids, nil
}

// min helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
