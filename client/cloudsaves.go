package client

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	netURL "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GOG Galaxy keeps game saves in a per-game cloud bucket. Reaching it takes
// three steps Lutris worked out: the game's build manifest names the game's
// own OAuth client, the user's refresh token is exchanged for a token scoped
// to that client, and the bucket is then a plain object store. Files in it
// are gzip-compressed and carry their original modification time in a
// header. Gogg only ever reads the bucket: backup must not be able to damage
// what the cloud holds.

// galaxyUserAgent is required by the cloud storage API; requests with other
// agents are refused.
const galaxyUserAgent = "GOGGalaxyCommunicationService/2.0.13.27 (Windows_32bit) dont_sync_marker/true installation_source/gog"

// emptyGzipMD5 marks a file GOG Galaxy wrote as an empty placeholder; there
// is nothing in it worth backing up.
const emptyGzipMD5 = "aadd86936a80ee8a369579c3926f1b3c"

// ErrCloudSavesUnavailable says the game has no cloud saves to back up: no
// build manifest, no game client, or an empty bucket.
var ErrCloudSavesUnavailable = errors.New("no cloud saves are available for this game")

// CloudSaveFile is one file in a game's cloud storage bucket.
type CloudSaveFile struct {
	// Name is the bucket path, forward slashes, such as "__default/save1.dat".
	Name         string
	MD5          string
	LastModified time.Time
}

// CloudSaveClient reads GOG Galaxy cloud storage. The URL fields exist so
// tests can point every step at their own server; NewCloudSaveClient fills
// in the real endpoints.
type CloudSaveClient struct {
	HTTP             *http.Client
	AuthURL          string
	ContentSystemURL string
	RemoteConfigURL  string
	StorageURL       string
}

func NewCloudSaveClient() *CloudSaveClient {
	return &CloudSaveClient{
		HTTP:             &http.Client{Timeout: 30 * time.Second},
		AuthURL:          "https://auth.gog.com",
		ContentSystemURL: "https://content-system.gog.com",
		RemoteConfigURL:  "https://remote-config.gog.com",
		StorageURL:       "https://cloudstorage.gog.com",
	}
}

// BackupResult says what a backup brought home.
type BackupResult struct {
	// Files are the bucket paths written under the output directory.
	Files []string
	// Skipped are bucket entries left out: empty placeholders and names
	// that would have escaped the output directory.
	Skipped []string
}

// BackupCloudSaves downloads every file in a game's cloud save bucket into
// outputDir, keeping the bucket's own layout and each file's modification
// time. The user's refresh token is needed because the bucket only answers
// to a token scoped to the game's own OAuth client.
func (c *CloudSaveClient) BackupCloudSaves(ctx context.Context, refreshToken string, gameID int, platform, outputDir string) (BackupResult, error) {
	var result BackupResult

	clientID, clientSecret, err := c.gameCredentials(ctx, gameID, platform)
	if err != nil {
		return result, err
	}
	accessToken, userID, err := c.gameToken(ctx, refreshToken, clientID, clientSecret)
	if err != nil {
		return result, fmt.Errorf("could not get a game-scoped token: %w", err)
	}

	files, err := c.listFiles(ctx, accessToken, userID, clientID)
	if err != nil {
		return result, err
	}
	if len(files) == 0 {
		return result, ErrCloudSavesUnavailable
	}

	if err := ensureDirExists(outputDir); err != nil {
		return result, err
	}
	cleanRoot := filepath.Clean(outputDir)
	for _, file := range files {
		if file.MD5 == emptyGzipMD5 {
			result.Skipped = append(result.Skipped, file.Name)
			continue
		}
		destination := filepath.Join(cleanRoot, filepath.FromSlash(file.Name))
		if !strings.HasPrefix(destination, cleanRoot+string(os.PathSeparator)) {
			log.Warn().Str("name", file.Name).Msg("Skipping cloud save entry: its name escapes the output directory")
			result.Skipped = append(result.Skipped, file.Name)
			continue
		}
		if err := c.downloadSave(ctx, accessToken, userID, clientID, file, destination); err != nil {
			return result, fmt.Errorf("failed to back up %s: %w", file.Name, err)
		}
		result.Files = append(result.Files, file.Name)
	}
	return result, nil
}

// gameCredentials reads the game's OAuth client from its build manifest on
// the content system. Builds are looked up per platform, and a game with no
// builds anywhere has nothing in the cloud either.
func (c *CloudSaveClient) gameCredentials(ctx context.Context, gameID int, platform string) (clientID, clientSecret string, err error) {
	// The content system spells the Apple platform "osx" where the rest of
	// gogg says "mac".
	platforms := []string{platform}
	switch platform {
	case "", "all":
		platforms = []string{"windows", "osx", "linux"}
	case "mac":
		platforms = []string{"osx"}
	}

	var builds struct {
		Items []struct {
			Link string `json:"link"`
			URLs []struct {
				URL string `json:"url"`
			} `json:"urls"`
		} `json:"items"`
	}
	manifestURL := ""
	for _, plat := range platforms {
		url := fmt.Sprintf("%s/products/%d/os/%s/builds?generation=2", c.ContentSystemURL, gameID, plat)
		body, err := c.get(ctx, url, "", "")
		if err != nil || json.Unmarshal(body, &builds) != nil || len(builds.Items) == 0 {
			continue
		}
		manifestURL = builds.Items[0].Link
		if manifestURL == "" && len(builds.Items[0].URLs) > 0 {
			manifestURL = builds.Items[0].URLs[0].URL
		}
		if manifestURL != "" {
			break
		}
	}
	if manifestURL == "" {
		return "", "", ErrCloudSavesUnavailable
	}

	raw, err := c.get(ctx, manifestURL, "", "")
	if err != nil {
		return "", "", fmt.Errorf("could not fetch the build manifest: %w", err)
	}
	// Generation 2 manifests come zlib-compressed; older ones are plain.
	if inflated, zerr := inflate(raw); zerr == nil {
		raw = inflated
	}
	var manifest struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return "", "", fmt.Errorf("could not parse the build manifest: %w", err)
	}
	if manifest.ClientID == "" {
		return "", "", ErrCloudSavesUnavailable
	}
	return manifest.ClientID, manifest.ClientSecret, nil
}

func inflate(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = reader.Close() }()
	return io.ReadAll(reader)
}

// gameToken exchanges the user's refresh token for one scoped to the game's
// client, which is the only token the storage bucket accepts.
func (c *CloudSaveClient) gameToken(ctx context.Context, refreshToken, clientID, clientSecret string) (accessToken, userID string, err error) {
	params := netURL.Values{
		"client_id":           {clientID},
		"client_secret":       {clientSecret},
		"grant_type":          {"refresh_token"},
		"refresh_token":       {refreshToken},
		"without_new_session": {"1"},
	}
	body, err := c.get(ctx, c.AuthURL+"/token?"+params.Encode(), "", "")
	if err != nil {
		return "", "", err
	}
	var token struct {
		AccessToken string `json:"access_token"`
		UserID      string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &token); err != nil {
		return "", "", err
	}
	if token.AccessToken == "" || token.UserID == "" {
		return "", "", fmt.Errorf("the token exchange returned no usable token")
	}
	return token.AccessToken, token.UserID, nil
}

// listFiles reads the bucket listing: every saved file with its hash and
// modification time.
func (c *CloudSaveClient) listFiles(ctx context.Context, accessToken, userID, clientID string) ([]CloudSaveFile, error) {
	url := fmt.Sprintf("%s/v1/%s/%s", c.StorageURL, userID, clientID)
	body, err := c.get(ctx, url, accessToken, "application/json")
	if err != nil {
		// Games that never enabled cloud storage answer 400; a bucket that
		// was never written answers 404. Both mean there are no saves, as
		// the probe against the live service showed.
		var status *httpStatusError
		if errors.As(err, &status) && (status.status == http.StatusNotFound || status.status == http.StatusBadRequest) {
			return nil, nil
		}
		return nil, err
	}
	var entries []struct {
		Name         string `json:"name"`
		Hash         string `json:"hash"`
		LastModified string `json:"last_modified"`
	}
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("could not parse the bucket listing: %w", err)
	}
	files := make([]CloudSaveFile, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "" {
			continue
		}
		modified, _ := time.Parse(time.RFC3339, entry.LastModified)
		files = append(files, CloudSaveFile{Name: entry.Name, MD5: entry.Hash, LastModified: modified})
	}
	return files, nil
}

// downloadSave fetches one bucket object, gunzips it, writes it under its
// bucket path, and restores the modification time Galaxy recorded for it.
func (c *CloudSaveClient) downloadSave(ctx context.Context, accessToken, userID, clientID string, file CloudSaveFile, destination string) error {
	segments := strings.Split(file.Name, "/")
	for i, segment := range segments {
		segments[i] = netURL.PathEscape(segment)
	}
	url := fmt.Sprintf("%s/v1/%s/%s/%s", c.StorageURL, userID, clientID, strings.Join(segments, "/"))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", galaxyUserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &httpStatusError{status: resp.StatusCode}
	}

	// The transport already gunzips when the server marks the body; a body
	// that still starts with the gzip magic is decompressed here.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		reader, gerr := gzip.NewReader(strings.NewReader(string(data)))
		if gerr == nil {
			if plain, rerr := io.ReadAll(reader); rerr == nil {
				data = plain
			}
			_ = reader.Close()
		}
	}

	if err := ensureDirExists(filepath.Dir(destination)); err != nil {
		return err
	}
	if err := os.WriteFile(destination, data, 0644); err != nil {
		return err
	}

	if stamp := resp.Header.Get("X-Object-Meta-LocalLastModified"); stamp != "" {
		if when, perr := time.Parse(time.RFC3339, stamp); perr == nil {
			_ = os.Chtimes(destination, when, when)
		}
	} else if !file.LastModified.IsZero() {
		_ = os.Chtimes(destination, file.LastModified, file.LastModified)
	}
	return nil
}

// get reads one small response, with the Galaxy agent on every call so the
// storage endpoints do not turn it away.
func (c *CloudSaveClient) get(ctx context.Context, url, accessToken, accept string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", galaxyUserAgent)
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{status: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}
