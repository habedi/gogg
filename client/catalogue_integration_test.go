//go:build integration

package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memTokenStore struct{}

func (memTokenStore) GetTokenRecord() (*db.Token, error) {
	return &db.Token{AccessToken: "tok", RefreshToken: "ref", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}, nil
}
func (memTokenStore) UpsertTokenRecord(token *db.Token) error { return nil }

type staticRefresher struct{}

func (staticRefresher) PerformTokenRefresh(refreshToken string) (string, string, int64, error) {
	return "tok", "ref", 3600, nil
}

func setupMemDB(t *testing.T) {
	gormDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open mem db: %v", err)
	}
	db.Db = gormDB
	if err := db.Db.AutoMigrate(&db.Token{}, &db.Game{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func TestIntegration_RefreshCatalogue_WithPaginationAndEdgeCases(t *testing.T) {
	setupMemDB(t)

	ownedPage1 := []int{1, 2}
	ownedPage2 := []int{3}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user/data/games":
			if r.URL.Query().Get("page") == "2" {
				json.NewEncoder(w).Encode(map[string]interface{}{"owned": ownedPage2})
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{
				"owned": ownedPage1,
				"next":  "/user/data/games?page=2",
			})
		case "/account/gameDetails/1.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"title": "Game One", "downloads": [][]interface{}{}})
		case "/account/gameDetails/2.json":
			json.NewEncoder(w).Encode(map[string]interface{}{"downloads": [][]interface{}{}})
		case "/account/gameDetails/3.json":
			list := make([][]interface{}, 3)
			for i := 0; i < 3; i++ {
				list[i] = []interface{}{"English", map[string]interface{}{"windows": []map[string]interface{}{{"name": "setup.exe", "size": "1GB"}}}}
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"title": "Game Three", "downloads": list})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	os.Setenv("GOGG_EMBED_BASE", server.URL)
	defer os.Unsetenv("GOGG_EMBED_BASE")

	st := memTokenStore{}
	rf := staticRefresher{}
	svc := auth.NewService(st, rf)

	repo := db.NewGameRepository(db.GetDB())
	ctx := context.Background()
	err := RefreshCatalogue(ctx, svc, repo, 3, nil)
	if err != nil && err != context.Canceled {
		t.Fatalf("refresh failed: %v", err)
	}

	g1, err := db.GetGameByID(1)
	if err != nil || g1 == nil || g1.Title != "Game One" {
		t.Fatalf("game 1 not stored correctly: %+v err=%v", g1, err)
	}
	g3, err := db.GetGameByID(3)
	if err != nil || g3 == nil || g3.Title != "Game Three" {
		t.Fatalf("game 3 not stored correctly: %+v err=%v", g3, err)
	}
	g2, err := db.GetGameByID(2)
	if err != nil {
		t.Fatalf("get 2: %v", err)
	}
	if g2 != nil && g2.Title != "" {
		t.Fatalf("game 2 should have empty or missing title, got: %+v", g2)
	}
}
