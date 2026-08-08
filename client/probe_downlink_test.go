//go:build probe

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
)

// A one-off probe against the real GOG API, run by hand with -tags probe.
// It answers one question: what shape does the downlink endpoint have, so
// checksum verification is built on fact rather than on guesswork.
func TestProbeDownlink(t *testing.T) {
	if err := db.ConfigurePathErr(); err != nil {
		t.Fatal(err)
	}
	if err := db.InitDB(); err != nil {
		t.Fatal(err)
	}
	defer db.Shutdown()

	service := auth.NewServiceWithRepo(db.NewTokenRepository(db.GetDB()), &GogClient{TokenURL: "https://auth.gog.com/token"})
	token, err := service.RefreshTokenCtx(context.Background())
	if err != nil {
		t.Skipf("no usable token: %v", err)
	}

	var games []db.Game
	if err := db.GetDB().Limit(30).Find(&games).Error; err != nil {
		t.Fatal(err)
	}

	get := func(url string) (int, []byte) {
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", url, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return resp.StatusCode, body
	}

	probed := 0
	for _, g := range games {
		game, err := ParseGameData(g.Data)
		if err != nil {
			continue
		}
		for _, d := range game.Downloads {
			for _, f := range d.Platforms.Windows {
				if f.ManualURL == nil || !strings.Contains(*f.ManualURL, "installer") {
					continue
				}
				parts := strings.Split(strings.Trim(*f.ManualURL, "/"), "/")
				fileID := parts[len(parts)-1]
				t.Logf("game=%d title=%q manualUrl=%s fileID=%s", g.ID, game.Title, *f.ManualURL, fileID)

				for _, dtype := range []string{"installer", "installers"} {
					url := fmt.Sprintf("https://api.gog.com/products/%d/downlink/%s/%s", g.ID, dtype, fileID)
					status, body := get(url)
					t.Logf("  %s -> %d %s", url, status, body)
					if status != http.StatusOK {
						continue
					}
					var payload struct {
						Downlink string `json:"downlink"`
						Checksum string `json:"checksum"`
					}
					if json.Unmarshal(body, &payload) != nil || payload.Checksum == "" {
						continue
					}
					cs, xml := get(payload.Checksum)
					t.Logf("  checksum %s -> %d %s", payload.Checksum, cs, xml)
				}
				probed++
				if probed >= 3 {
					return
				}
			}
		}
	}
	if probed == 0 {
		t.Skip("no installer manual URLs in the first games")
	}
}
