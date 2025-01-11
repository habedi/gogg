package client

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

// FetchGameData retrieves the game data for the specified game from GOG.
func FetchGameData(accessToken string, url string) (Game, string, error) {
	//url := fmt.Sprintf("https://embed.gog.com/account/gameDetails/%d.json", gameID)
	req, err := createRequest("GET", url, accessToken)
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

// FetchIdOfOwnedGames retrieves the list of game IDs that the user owns from GOG.
func FetchIdOfOwnedGames(accessToken string, apiURL string) ([]int, error) {
	//apiURL := "https://embed.gog.com/user/data/games"
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

// createRequest creates an HTTP request with the specified method, URL, and access token from GOG.
func createRequest(method, url, accessToken string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
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

// readResponseBody reads the response body and returns it as a byte slice.
func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		return nil, err
	}
	return body, nil
}

// parseGameData parses the game data from the response body.
func parseGameData(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return err
	}
	return nil
}

// parseOwnedGames parses the list of owned game IDs from the response body.
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
