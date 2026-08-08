package gui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

type stubTokenStore struct{ token *db.Token }

func (s *stubTokenStore) GetTokenRecord() (*db.Token, error)  { return s.token, nil }
func (s *stubTokenStore) UpsertTokenRecord(_ *db.Token) error { return nil }

type stubRefresher struct{}

func (stubRefresher) PerformTokenRefresh(_ string) (string, string, int64, error) {
	return "access", "refresh", 3600, nil
}

func stubAuthService() *auth.Service {
	return auth.NewService(
		&stubTokenStore{token: &db.Token{
			AccessToken:  "access",
			RefreshToken: "refresh",
			ExpiresAt:    time.Now().Add(time.Hour).Format(time.RFC3339),
		}},
		stubRefresher{},
	)
}

// The download manager must never run more downloads at once than the
// configured limit, however fast the queue is drained.
func TestStartNextIfAvailable_RespectsMaxConcurrent(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetInt("download.maxConcurrent", 2)

	// Downloads block until the test releases them, so started downloads stay
	// active while the queue is being drained.
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "4")
			return
		}
		<-release
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	downloadPath := t.TempDir()

	const queued = 5
	for i := 1; i <= queued; i++ {
		data := fmt.Sprintf(
			`{"title":"Game %d","downloads":[["English",{"windows":[{"manualUrl":%q,"name":"file.bin","size":"1 MB"}]}]],"extras":[],"dlcs":[]}`,
			i, srv.URL+"/file.bin")
		dm.queue = append(dm.queue, queuedDownload{
			authService:  stubAuthService(),
			game:         db.Game{ID: i, Title: fmt.Sprintf("Game %d", i), Data: data},
			downloadPath: downloadPath,
			language:     "English",
			platformName: "windows",
			numThreads:   1,
		})
	}

	defer func() {
		dm.mu.Lock()
		dm.queue = nil
		dm.mu.Unlock()
		close(release)
		// Let the in-flight downloads unwind before the server goes away.
		require.Eventually(t, func() bool {
			dm.mu.RLock()
			defer dm.mu.RUnlock()
			tasks, _ := dm.Tasks.Get()
			for _, raw := range tasks {
				switch raw.(*DownloadTask).State() {
				case StateCompleted, StateCancelled, StateError:
				default:
					return false
				}
			}
			return true
		}, 5*time.Second, 10*time.Millisecond)
	}()

	dm.startNextIfAvailable()

	dm.mu.RLock()
	remaining := len(dm.queue)
	dm.mu.RUnlock()
	require.Equal(t, queued-2, remaining, "only maxConcurrent downloads may be started")

	tasks, err := dm.Tasks.Get()
	require.NoError(t, err)
	require.Len(t, tasks, 2, "each started download must be visible as a task right away")
}

// A game whose stored data cannot be parsed fails loudly instead of silently
// occupying a download slot.
func TestExecuteDownload_ReportsUnparseableGameData(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	err := executeDownload(dm, queuedDownload{
		authService:  stubAuthService(),
		game:         db.Game{ID: 1, Title: "Broken", Data: "{not json"},
		downloadPath: t.TempDir(),
		language:     "English",
		platformName: "windows",
		numThreads:   1,
	})
	require.Error(t, err)

	// The slot must be released so the game can be retried.
	require.False(t, dm.slotHeld(1))
}

// A download of several languages and platforms runs as one task, and what it
// was made of is written down beside the files.
func TestExecuteDownload_CoversEveryTickedChoice(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Preferences().SetBool("soundEnabled", false)
	app.Preferences().SetBool(prefNotifications, false)

	root := t.TempDir()
	dm := &DownloadManager{Tasks: binding.NewUntypedList()}
	require.NoError(t, executeDownload(dm, queuedDownload{
		authService:  stubAuthService(),
		game:         db.Game{ID: 12, Title: "Every Way", Data: `{"title":"Every Way","downloads":[],"extras":[],"dlcs":[]}`},
		downloadPath: root,
		language:     "English", platformName: "all",
		languages: []string{"English", "Deutsch"}, platforms: []string{"windows", "linux"},
		numThreads: 1,
	}))

	tasks := dm.tasksSnapshot()
	require.Len(t, tasks, 1)
	task := tasks[0]
	require.Eventually(t, func() bool { return task.State() == StateCompleted },
		5*time.Second, 10*time.Millisecond)

	info, err := os.ReadFile(filepath.Join(root, client.SanitizePath("Every Way"), "download_info.json"))
	require.NoError(t, err)
	var written struct {
		Languages []string `json:"languages"`
		Platforms []string `json:"platforms"`
	}
	require.NoError(t, json.Unmarshal(info, &written))
	require.Equal(t, []string{"English", "Deutsch"}, written.Languages)
	require.Equal(t, []string{"windows", "linux"}, written.Platforms)
}
