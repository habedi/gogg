package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

var sizeRegexp = regexp.MustCompile(`(?i)^\s*([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]+)?\s*$`)

func parseSizeString(sizeStr string) (int64, error) {
	s := strings.TrimSpace(sizeStr)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}
	m := sizeRegexp.FindStringSubmatch(s)
	if len(m) == 0 {
		// Try pure integer bytes
		if v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil {
			return v, nil
		}
		return 0, fmt.Errorf("unable to parse size '%s'", sizeStr)
	}
	valStr := m[1]
	unit := strings.ToLower(m[2])
	if unit == "" {
		// Bytes without unit
		v, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			return 0, err
		}
		return int64(v), nil
	}
	// Normalize common units
	switch unit {
	case "b", "bytes":
		unit = "b"
	case "k", "kb", "kib":
		unit = "kb"
	case "m", "mb", "mib":
		unit = "mb"
	case "g", "gb", "gib":
		unit = "gb"
	case "t", "tb", "tib":
		unit = "tb"
	}
	var mult float64
	switch unit {
	case "b":
		mult = 1
	case "kb":
		mult = 1024
	case "mb":
		mult = 1024 * 1024
	case "gb":
		mult = 1024 * 1024 * 1024
	case "tb":
		mult = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unknown size unit '%s' in '%s'", unit, sizeStr)
	}
	v, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return 0, err
	}
	return int64(v * mult), nil
}

func FetchGameData(ctx context.Context, accessToken string, url string) (Game, string, error) {
	req, err := createRequest(ctx, "GET", url, accessToken)
	if err != nil {
		return Game{}, "", err
	}

	resp, err := sendRequest(req)
	if err != nil {
		return Game{}, "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Debug().Err(err).Msg("Failed to close response body")
		}
	}()

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

func FetchIdOfOwnedGames(ctx context.Context, accessToken string, apiURL string) ([]int, error) {
	req, err := createRequest(ctx, "GET", apiURL, accessToken)
	if err != nil {
		return nil, err
	}

	resp, err := sendRequest(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Debug().Err(err).Msg("Failed to close response body")
		}
	}()

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	return parseOwnedGames(body)
}

func createRequest(ctx context.Context, method, url, accessToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
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
			closeResponseBody(resp)
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
		closeResponseBody(resp)
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

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.CopyN(io.Discard, resp.Body, 1024*1024)
	_ = resp.Body.Close()
}

func parseGameData(body []byte, game *Game) error {
	if err := json.Unmarshal(body, game); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return err
	}
	return nil
}

type ownedResponse struct {
	Owned []int  `json:"owned"`
	Next  string `json:"next,omitempty"`
}

func resolveNext(baseURL, next string) string {
	if next == "" {
		return ""
	}
	if u, err := url.Parse(next); err == nil && u.Scheme != "" && u.Host != "" {
		return next
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return next
	}
	n, err := url.Parse(next)
	if err != nil {
		return next
	}
	return base.ResolveReference(n).String()
}

func canonicalizeURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	// normalize trailing slash in path
	if parsed.Path == "/" {
		parsed.Path = ""
	} else {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	parsed.RawQuery = strings.TrimSuffix(parsed.RawQuery, "&")
	return parsed.String()
}

func FetchAllOwnedGameIDs(ctx context.Context, accessToken, startURL string) ([]int, error) {
	all := make([]int, 0, 128)
	nextURL := canonicalizeURL(startURL)
	seen := map[string]bool{}
	for nextURL != "" {
		key := canonicalizeURL(nextURL)
		if seen[key] {
			break
		}
		seen[key] = true

		req, err := http.NewRequestWithContext(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		resp, err := sendRequest(req)
		if err != nil {
			return nil, err
		}
		func() {
			defer func() { _ = resp.Body.Close() }()
			body, err := readResponseBody(resp)
			if err != nil {
				nextURL = ""
				return
			}
			var or ownedResponse
			if err := json.Unmarshal(body, &or); err != nil {
				nextURL = ""
				return
			}
			all = append(all, or.Owned...)
			resolved := resolveNext(nextURL, or.Next)
			nextURL = canonicalizeURL(resolved)
		}()
	}
	return all, nil
}

func parseOwnedGames(body []byte) ([]int, error) {
	var response ownedResponse
	if err := json.Unmarshal(body, &response); err != nil {
		log.Error().Err(err).Msg("Failed to parse response")
		return nil, err
	}
	return response.Owned, nil
}

func (g *Game) EstimateStorageSize(language, platformName string, extrasFlag, dlcFlag bool) (int64, error) {
	var totalSizeBytes int64

	processFiles := func(files []PlatformFile) {
		for _, file := range files {
			if size, err := parseSizeString(file.Size); err == nil {
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
			if size, err := parseSizeString(extra.Size); err == nil {
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
					if size, err := parseSizeString(extra.Size); err == nil {
						totalSizeBytes += size
					}
				}
			}
		}
	}

	return totalSizeBytes, nil
}
