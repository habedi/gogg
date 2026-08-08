//go:build probe

package client

import (
	"context"
	"testing"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
)

// A one-off probe against the real GOG services, run by hand with -tags
// probe. It walks the cloud save chain for a few owned games and reports
// what each step answered, so the backup rests on observed behavior.
func TestProbeCloudSaves(t *testing.T) {
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
	if err := db.GetDB().Limit(40).Find(&games).Error; err != nil {
		t.Fatal(err)
	}

	saves := NewCloudSaveClient()
	ctx := context.Background()
	for _, g := range games {
		clientID, _, err := saves.gameCredentials(ctx, g.ID, "windows")
		if err != nil {
			t.Logf("game=%d %q: no credentials (%v)", g.ID, g.Title, err)
			continue
		}
		accessToken, userID, err := saves.gameToken(ctx, token.RefreshToken, clientID, "")
		if err != nil {
			// Retry with the secret in place, which the real exchange needs.
			cid, secret, cerr := saves.gameCredentials(ctx, g.ID, "windows")
			if cerr == nil {
				accessToken, userID, err = saves.gameToken(ctx, token.RefreshToken, cid, secret)
			}
		}
		if err != nil {
			t.Logf("game=%d %q: client=%s token exchange failed: %v", g.ID, g.Title, clientID, err)
			continue
		}
		files, err := saves.listFiles(ctx, accessToken, userID, clientID)
		if err != nil {
			t.Logf("game=%d %q: client=%s user=%s listing failed: %v", g.ID, g.Title, clientID, userID, err)
			continue
		}
		t.Logf("game=%d %q: client=%s user=%s files=%d", g.ID, g.Title, clientID, userID, len(files))
		for i, f := range files {
			if i >= 5 {
				t.Logf("  ...and %d more", len(files)-5)
				break
			}
			t.Logf("  %s hash=%s modified=%s", f.Name, f.MD5, f.LastModified)
		}
	}
}
