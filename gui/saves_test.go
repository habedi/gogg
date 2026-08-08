package gui

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

// fakeBackuper answers the backup call without any network and records what
// it was asked, delivering through a channel so the test can wait for the
// background goroutine instead of racing it.
type fakeBackuper struct {
	mu        sync.Mutex
	gameID    int
	outputDir string
	result    client.BackupResult
	err       error
	called    chan struct{}
}

func (f *fakeBackuper) BackupCloudSaves(_ context.Context, _ string, gameID int, _, outputDir string) (client.BackupResult, error) {
	f.mu.Lock()
	f.gameID = gameID
	f.outputDir = outputDir
	f.mu.Unlock()
	defer close(f.called)
	return f.result, f.err
}

// installFakeBackuper swaps the cloud client seam for one test, along with a
// notification recorder that delivers through a channel.
func installFakeBackuper(t *testing.T, fake *fakeBackuper) chan string {
	t.Helper()
	fake.called = make(chan struct{})
	originalClient := newCloudSaveClient
	newCloudSaveClient = func() cloudSaveBackuper { return fake }
	t.Cleanup(func() { newCloudSaveClient = originalClient })

	notes := make(chan string, 2)
	originalNotify := notify
	notify = func(title, content string) { notes <- title + ": " + content }
	t.Cleanup(func() { notify = originalNotify })
	return notes
}

func awaitNote(t *testing.T, notes chan string) string {
	t.Helper()
	select {
	case note := <-notes:
		return note
	case <-time.After(5 * time.Second):
		t.Fatal("no notification arrived")
		return ""
	}
}

func TestBackupSavesButton_ReportsWhatItBroughtHome(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	fake := &fakeBackuper{result: client.BackupResult{Files: []string{"__default/a.sav", "__default/b.sav"}}}
	notes := installFakeBackuper(t, fake)

	lt, root := newLibraryFixture(t, 1)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: `{"title":"Game 1"}`}))

	button := buttonWithLabel(lt.content, "Saves")
	require.NotNil(t, button, "the pane offers the backup button")
	button.OnTapped()

	note := awaitNote(t, notes)
	require.Contains(t, note, "Backed up 2 save files for Game 1")

	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, 1, fake.gameID)
	require.Equal(t, filepath.Join(root, "saves", "game-1"), fake.outputDir,
		"the saves land under the download path, beside the games")
}

func TestBackupSavesButton_SaysWhenThereIsNothing(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	fake := &fakeBackuper{err: client.ErrCloudSavesUnavailable}
	notes := installFakeBackuper(t, fake)

	lt, _ := newLibraryFixture(t, 1)
	require.NoError(t, lt.selected.Set(db.Game{ID: 1, Title: "Game 1", Data: `{"title":"Game 1"}`}))

	buttonWithLabel(lt.content, "Saves").OnTapped()

	require.Contains(t, awaitNote(t, notes), "No cloud saves found for Game 1")
}

func TestBackupSavesButton_DoesNothingWithoutAGame(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	fake := &fakeBackuper{}
	installFakeBackuper(t, fake)

	lt, _ := newLibraryFixture(t, 1)
	buttonWithLabel(lt.content, "Saves").OnTapped()

	select {
	case <-fake.called:
		t.Fatal("no game is selected, so nothing must be asked of the cloud")
	case <-time.After(50 * time.Millisecond):
	}
}

// The real backup engine and the button agree on the contract: this is the
// same fixture the client tests use, driven from the widget.
func TestBackupSavesButton_EndToEndAgainstTheFixture(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	fx := newGalaxySaveFixture(t, map[string][]byte{"__default/slot.sav": []byte("save bytes")})
	originalClient := newCloudSaveClient
	newCloudSaveClient = func() cloudSaveBackuper { return fx }
	t.Cleanup(func() { newCloudSaveClient = originalClient })

	notes := make(chan string, 2)
	originalNotify := notify
	notify = func(title, content string) { notes <- content }
	t.Cleanup(func() { notify = originalNotify })

	lt, root := newLibraryFixture(t, 1)
	require.NoError(t, lt.selected.Set(db.Game{ID: 42, Title: "Game 1", Data: `{"title":"Game 1"}`}))

	buttonWithLabel(lt.content, "Saves").OnTapped()

	require.Contains(t, awaitNote(t, notes), "Backed up 1 save file")
	saved, err := os.ReadFile(filepath.Join(root, "saves", "game-1", "__default", "slot.sav"))
	require.NoError(t, err)
	require.Equal(t, []byte("save bytes"), saved)
}

// newGalaxySaveFixture stands in for the GOG services the real cloud client
// talks to, serving one bucket of saves for any game asked about.
func newGalaxySaveFixture(t *testing.T, saves map[string][]byte) *client.CloudSaveClient {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/os/windows/builds"):
			_, _ = fmt.Fprintf(w, `{"items":[{"link":"%s/manifest"}]}`, srv.URL)
		case strings.Contains(path, "/builds"):
			_, _ = w.Write([]byte(`{"items":[]}`))
		case path == "/manifest":
			_, _ = w.Write([]byte(`{"clientId":"game-client","clientSecret":"s"}`))
		case path == "/token":
			_, _ = w.Write([]byte(`{"access_token":"scoped","user_id":"u1"}`))
		case path == "/v1/u1/game-client":
			entries := make([]map[string]string, 0, len(saves))
			for name := range saves {
				entries = append(entries, map[string]string{"name": name, "hash": "h"})
			}
			_ = json.NewEncoder(w).Encode(entries)
		case strings.HasPrefix(path, "/v1/u1/game-client/"):
			content, ok := saves[strings.TrimPrefix(path, "/v1/u1/game-client/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			_, _ = gz.Write(content)
			_ = gz.Close()
			_, _ = w.Write(buf.Bytes())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return &client.CloudSaveClient{
		HTTP:             srv.Client(),
		AuthURL:          srv.URL,
		ContentSystemURL: srv.URL,
		RemoteConfigURL:  srv.URL,
		StorageURL:       srv.URL,
	}
}
