package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

func FetchGameData(accessToken string, url string) (Game, string, error) {
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

func createRequest(method, url, accessToken string) (*http.Request, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request")
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	return req, nil
}

func sendRequest(req *http.Request) (*http.Response, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	var resp *http.Response
	var err error

	const maxRetries = 3
	backoff := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		resp, err = client.Do(req)
		if err != nil {
			log.Warn().Err(err).Int("attempt", i+1).Int("max_attempts", maxRetries).Msg("Request failed, retrying...")
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		if resp.StatusCode >= 500 {
			log.Warn().Int("status", resp.StatusCode).Int("attempt", i+1).Int("max_attempts", maxRetries).Msg("Server error, retrying...")
			time.Sleep(backoff)
			backoff *= 2
			continue
		}

		break
	}

	if err != nil {
		log.Error().Err(err).Msg("Failed to send request after multiple retries")
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Error().Int("status", resp.StatusCode).Msg("HTTP request failed with non-successful status")
		return nil, fmt.Errorf("HTTP request failed with status %d", resp.StatusCode)
	}
	return resp, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		return nil, err
	}
	return body, nil
}

func parseGameData(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return err
	}
	return nil
}

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

func (g *Game) EstimateStorageSize(language, platformName string, extrasFlag, dlcFlag bool) (int64, error) {
	var totalSizeBytes int64

	parseSize := func(sizeStr string) (int64, error) {
		s := strings.TrimSpace(strings.ToLower(sizeStr))
		var val float64
		var err error
		switch {
		case strings.HasSuffix(s, " gb"):
			_, err = fmt.Sscanf(s, "%f gb", &val)
			if err != nil {
				return 0, err
			}
			return int64(val * 1024 * 1024 * 1024), nil
		case strings.HasSuffix(s, " mb"):
			_, err = fmt.Sscanf(s, "%f mb", &val)
			if err != nil {
				return 0, err
			}
			return int64(val * 1024 * 1024), nil
		case strings.HasSuffix(s, " kb"):
			_, err = fmt.Sscanf(s, "%f kb", &val)
			if err != nil {
				return 0, err
			}
			return int64(val * 1024), nil
		default:
			bytesVal, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				return bytesVal, nil
			}
			return 0, fmt.Errorf("unknown or missing size unit in '%s'", sizeStr)
		}
	}

	processFiles := func(files []PlatformFile) {
		for _, file := range files {
			if size, err := parseSize(file.Size); err == nil {
				totalSizeBytes += size
			}
		}
	}

	for _, download := range g.Downloads {
		if !strings.EqualFold(download.Language, language) {
			continue
		}
		platforms := map[string][]PlatformFile{
			"windows": download.Platforms.Windows,
			"mac":     download.Platforms.Mac,
			"linux":   download.Platforms.Linux,
		}
		for name, files := range platforms {
			if platformName == "all" || strings.EqualFold(platformName, name) {
				processFiles(files)
			}
		}
	}

	if extrasFlag {
		for _, extra := range g.Extras {
			if size, err := parseSize(extra.Size); err == nil {
				totalSizeBytes += size
			}
		}
	}

	if dlcFlag {
		for _, dlc := range g.DLCs {
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, language) {
					continue
				}
				platforms := map[string][]PlatformFile{
					"windows": download.Platforms.Windows,
					"mac":     download.Platforms.Mac,
					"linux":   download.Platforms.Linux,
				}
				for name, files := range platforms {
					if platformName == "all" || strings.EqualFold(platformName, name) {
						processFiles(files)
					}
				}
			}
			if extrasFlag {
				for _, extra := range dlc.Extras {
					if size, err := parseSize(extra.Size); err == nil {
						totalSizeBytes += size
					}
				}
			}
		}
	}

	return totalSizeBytes, nil
}
