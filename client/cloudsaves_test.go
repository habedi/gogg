package client

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// galaxyFixture plays every GOG service the backup talks to: the content
// system with its builds and manifest, the token exchange, and the storage
// bucket itself.
type galaxyFixture struct {
	srv     *httptest.Server
	saves   map[string][]byte // bucket path -> plain content
	hashes  map[string]string // bucket path -> listed hash
	modTime time.Time
}

func newGalaxyFixture(t *testing.T) *galaxyFixture {
	t.Helper()
	fx := &galaxyFixture{
		saves:   map[string][]byte{},
		hashes:  map[string]string{},
		modTime: time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	}
	fx.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("User-Agent"), "GOGGalaxyCommunicationService") {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/os/windows/builds"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []map[string]any{{"link": fx.srv.URL + "/manifest"}},
			})
		case strings.Contains(path, "/os/osx/builds"), strings.Contains(path, "/os/linux/builds"):
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		case path == "/manifest":
			var buf bytes.Buffer
			zw := zlib.NewWriter(&buf)
			_, _ = zw.Write([]byte(`{"clientId":"game-client","clientSecret":"game-secret"}`))
			_ = zw.Close()
			_, _ = w.Write(buf.Bytes())
		case path == "/token":
			q := r.URL.Query()
			if q.Get("client_id") != "game-client" || q.Get("grant_type") != "refresh_token" ||
				q.Get("refresh_token") != "user-refresh" || q.Get("without_new_session") != "1" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "game-scoped-token",
				"user_id":      "user-1",
			})
		case path == "/v1/user-1/game-client":
			if r.Header.Get("Authorization") != "Bearer game-scoped-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			var entries []map[string]string
			for name := range fx.saves {
				entries = append(entries, map[string]string{
					"name":          name,
					"hash":          fx.hashes[name],
					"last_modified": fx.modTime.Format(time.RFC3339),
				})
			}
			_ = json.NewEncoder(w).Encode(entries)
		case strings.HasPrefix(path, "/v1/user-1/game-client/"):
			name := strings.TrimPrefix(path, "/v1/user-1/game-client/")
			content, ok := fx.saves[name]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write(content)
			_ = gz.Close()
			w.Header().Set("X-Object-Meta-LocalLastModified", fx.modTime.Format(time.RFC3339))
			_, _ = w.Write(buf.Bytes())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fx.srv.Close)
	return fx
}

func (fx *galaxyFixture) client() *CloudSaveClient {
	return &CloudSaveClient{
		HTTP:             fx.srv.Client(),
		AuthURL:          fx.srv.URL,
		ContentSystemURL: fx.srv.URL,
		RemoteConfigURL:  fx.srv.URL,
		StorageURL:       fx.srv.URL,
	}
}

func TestBackupCloudSaves_BringsTheBucketHome(t *testing.T) {
	fx := newGalaxyFixture(t)
	fx.saves["__default/slot1.sav"] = []byte("the actual save bytes")
	fx.saves["__default/nested/slot2.sav"] = []byte("a nested save")

	tmp := t.TempDir()
	result, err := fx.client().BackupCloudSaves(context.Background(), "user-refresh", 42, "windows", tmp)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"__default/slot1.sav", "__default/nested/slot2.sav"}, result.Files)
	require.Empty(t, result.Skipped)

	saved, err := os.ReadFile(filepath.Join(tmp, "__default", "slot1.sav"))
	require.NoError(t, err)
	require.Equal(t, []byte("the actual save bytes"), saved, "the file arrives decompressed")

	info, err := os.Stat(filepath.Join(tmp, "__default", "nested", "slot2.sav"))
	require.NoError(t, err)
	require.True(t, info.ModTime().Equal(fx.modTime),
		"the modification time Galaxy recorded survives the backup")
}

func TestBackupCloudSaves_SkipsPlaceholdersAndEscapees(t *testing.T) {
	fx := newGalaxyFixture(t)
	fx.saves["__default/real.sav"] = []byte("real")
	fx.saves["__default/empty.sav"] = []byte("")
	fx.hashes["__default/empty.sav"] = emptyGzipMD5
	fx.saves["../escape.sav"] = []byte("must never land outside")

	tmp := t.TempDir()
	result, err := fx.client().BackupCloudSaves(context.Background(), "user-refresh", 42, "windows", tmp)
	require.NoError(t, err)
	require.Equal(t, []string{"__default/real.sav"}, result.Files)
	require.ElementsMatch(t, []string{"__default/empty.sav", "../escape.sav"}, result.Skipped)

	_, statErr := os.Stat(filepath.Join(filepath.Dir(tmp), "escape.sav"))
	require.True(t, os.IsNotExist(statErr), "a hostile bucket name must not write outside the output directory")
}

func TestBackupCloudSaves_NothingThereIsSaidPlainly(t *testing.T) {
	fx := newGalaxyFixture(t)

	_, err := fx.client().BackupCloudSaves(context.Background(), "user-refresh", 42, "windows", t.TempDir())
	require.ErrorIs(t, err, ErrCloudSavesUnavailable, "an empty bucket is no saves, not a failure")

	fxNoBuilds := newGalaxyFixture(t)
	_, err = fxNoBuilds.client().BackupCloudSaves(context.Background(), "user-refresh", 42, "linux", t.TempDir())
	require.ErrorIs(t, err, ErrCloudSavesUnavailable, "a game with no builds has no cloud saves either")
}

func TestBackupCloudSaves_MacSpellsItsPlatformOsx(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/builds") {
			asked = append(asked, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	saves := &CloudSaveClient{
		HTTP:             srv.Client(),
		AuthURL:          srv.URL,
		ContentSystemURL: srv.URL,
		RemoteConfigURL:  srv.URL,
		StorageURL:       srv.URL,
	}
	_, err := saves.BackupCloudSaves(context.Background(), "user-refresh", 7, "mac", t.TempDir())
	require.ErrorIs(t, err, ErrCloudSavesUnavailable)
	require.Equal(t, []string{fmt.Sprintf("/products/%d/os/osx/builds", 7)}, asked)
}

func TestNewCloudSaveClient_PointsAtGOG(t *testing.T) {
	c := NewCloudSaveClient()
	require.NotNil(t, c)
	require.NotNil(t, c.HTTP)
	require.Equal(t, "https://auth.gog.com", c.AuthURL)
	require.Equal(t, "https://content-system.gog.com", c.ContentSystemURL)
	require.Equal(t, "https://remote-config.gog.com", c.RemoteConfigURL)
	require.Equal(t, "https://cloudstorage.gog.com", c.StorageURL)
}
