package client

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	netURL "net/url"
	"strings"

	"github.com/rs/zerolog/log"
)

// GOG publishes an official MD5 for most installer and patch files. The
// downlink endpoint returns, next to the file's real address, the address of
// a small XML manifest whose root element carries that MD5. Fetching it lets
// a download be verified against what GOG says the file is, not merely
// recorded as what arrived.

// downlinkKind classifies a file by the last segment of its manual URL, which
// doubles as the file's downlink ID. Only installers and patches have
// checksum manifests; anything else returns an empty kind and is not looked
// up.
func downlinkKind(manualURL string) (kind, fileID string) {
	parsed, err := netURL.Parse(manualURL)
	if err != nil {
		return "", ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	fileID = segments[len(segments)-1]
	lower := strings.ToLower(fileID)
	switch {
	case strings.Contains(lower, "installer"):
		return "installer", fileID
	case strings.Contains(lower, "patch"):
		return "patch", fileID
	}
	return "", ""
}

// fetchExpectedMD5 asks GOG for the official MD5 of one file. An empty result
// means no verdict rather than failure: the file has no manifest, the product
// ID does not serve it, or the network let the lookup down. A download must
// never fail because its verification data was unavailable.
func fetchExpectedMD5(ctx context.Context, httpClient *http.Client, accessToken string, productID int, manualURL string) string {
	kind, fileID := downlinkKind(manualURL)
	if kind == "" || productID == 0 {
		return ""
	}

	url := fmt.Sprintf("%s/products/%d/downlink/%s/%s", apiBase(), productID, kind, fileID)
	body, err := fetchWithBearer(ctx, httpClient, accessToken, url)
	if err != nil {
		log.Debug().Err(err).Str("url", url).Msg("Checksum lookup skipped: downlink request failed")
		return ""
	}
	var payload struct {
		Checksum string `json:"checksum"`
	}
	if json.Unmarshal(body, &payload) != nil || payload.Checksum == "" {
		return ""
	}

	manifest, err := fetchWithBearer(ctx, httpClient, accessToken, payload.Checksum)
	if err != nil {
		log.Debug().Err(err).Str("url", payload.Checksum).Msg("Checksum lookup skipped: manifest request failed")
		return ""
	}
	var root struct {
		MD5 string `xml:"md5,attr"`
	}
	if xml.Unmarshal(manifest, &root) != nil {
		return ""
	}
	return strings.ToLower(root.MD5)
}

// fetchWithBearer reads one small authenticated response, refusing anything
// that is not a plain 200.
func fetchWithBearer(ctx context.Context, httpClient *http.Client, accessToken, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

// checksumError is a download whose bytes do not match what GOG published.
// It is retryable: the copy on disk is deleted first, so the next attempt
// starts clean.
type checksumError struct {
	expected string
	got      string
}

func (e *checksumError) Error() string {
	return fmt.Sprintf("checksum mismatch: GOG lists %s, received %s", e.expected, e.got)
}
