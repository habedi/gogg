package gui

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

// updateStatus holds cached per-game status.
type updateStatus struct {
	Downloaded bool
	HasUpdate  bool
	Diff       []string // human-readable changes
	// ChangedAt is when gogg first noticed the update now waiting, so the
	// library can be asked for what changed recently.
	ChangedAt time.Time `json:",omitempty"`
}

// gameStatuses is what was found out about a set of games.
type gameStatuses = map[int]updateStatus

// statusWorker runs the work of finding out what has been downloaded and then
// hands what it found back to be recorded. Working it out reads the filesystem
// and parses every stored game, which is not work for the thread that draws the
// window, so by default the two halves run on different ones. It is a variable
// so tests can run both where they can watch them.
var statusWorker = func(work func() gameStatuses, apply func(gameStatuses)) {
	go func() {
		found := work()
		runOnMain(func() { apply(found) })
	}()
}

// computeUpdateStatus works the statuses out and records them, for callers that
// need the answer before they go on.
func computeUpdateStatus(s *libraryState, dm *DownloadManager, games []db.Game) {
	s.applyStatuses(statusesFor(dm, games))
}

// applyStatuses records what was found. The window reads these, so they are
// only ever written on the thread that draws it. An update carries the moment
// it was first noticed: the same one seen again keeps its date, a different
// one gets today's.
func (s *libraryState) applyStatuses(found gameStatuses) {
	for id, status := range found {
		if status.HasUpdate {
			previous := s.statuses[id]
			if previous.HasUpdate && !previous.ChangedAt.IsZero() && slices.Equal(previous.Diff, status.Diff) {
				status.ChangedAt = previous.ChangedAt
			} else {
				status.ChangedAt = time.Now()
			}
		}
		s.statuses[id] = status
	}
	s.persistStatuses()
}

// statusesFor works out what has been downloaded and what has changed since.
// It touches nothing the window reads, so it can run on a thread of its own.
func statusesFor(dm *DownloadManager, games []db.Game) gameStatuses {
	prefs := fyne.CurrentApp().Preferences()
	includeExtrasUpdates := prefs.BoolWithFallback("downloadForm.includeExtrasUpdates", false)
	includeDLCUpdates := prefs.BoolWithFallback("downloadForm.includeDLCUpdates", false)
	scanDirs := prefs.BoolWithFallback("downloadForm.scanDirsForDownloads", true)
	includePatchUpdates := prefs.BoolWithFallback("downloadForm.includePatchUpdates", false)
	langPref := formLanguage(prefs)
	platformPref := formPlatform(prefs)

	found := make(gameStatuses, len(games))
	for _, game := range games {
		// Downloaded determination
		downloaded := isGameDownloaded(dm, game.ID)
		var dir string
		if !downloaded && scanDirs {
			if d, ok := getGameDownloadDirectory(dm, game); ok {
				dir = d
				downloaded = true
			}
		}
		if downloaded && dir == "" {
			// history path if available
			if p, ok := getLastCompletedDownloadDir(dm, game.ID); ok {
				dir = p
			}
		}

		status := updateStatus{Downloaded: downloaded}
		if downloaded && dir != "" {
			oldMeta, err1 := readDownloadedMetadata(dm, game.ID)
			if err1 != nil && scanDirs { // try reading direct dir if fallback path differs
				metaPath := filepath.Join(dir, "metadata.json")
				if b, err2 := os.ReadFile(metaPath); err2 == nil {
					var gm client.Game
					if json.Unmarshal(b, &gm) == nil {
						oldMeta = &gm
					}
				}
			}
			current, err3 := client.ParseGameData(game.Data)
			if err3 == nil && oldMeta != nil {
				infoLang, infoPlatform := readDownloadInfo(dir)
				// The stored game data names languages in full, while the
				// preference holds a code such as "en".
				lang := client.GameLanguages[langPref]
				if lang == "" {
					lang = langPref
				}
				platform := platformPref
				if infoLang != "" {
					lang = infoLang
				}
				if infoPlatform != "" {
					platform = infoPlatform
				}
				oldMap := buildVersionMapExtended(*oldMeta, lang, platform, includeExtrasUpdates, includeDLCUpdates, includePatchUpdates)
				newMap := buildVersionMapExtended(current, lang, platform, includeExtrasUpdates, includeDLCUpdates, includePatchUpdates)
				diff := make([]string, 0)
				for k, newVer := range newMap {
					oldVer, ok := oldMap[k]
					if !ok {
						diff = append(diff, describeAddedFile(k, newVer))
					} else if newVer != oldVer {
						diff = append(diff, describeChangedFile(k, oldVer, newVer))
					}
				}
				// Map order is not an order, and a list that shuffles itself
				// every time it is worked out cannot be compared with itself.
				sort.Strings(diff)
				if len(diff) > 0 {
					status.HasUpdate = true
					status.Diff = diff
				}
			}
		}
		found[game.ID] = status
	}
	return found
}

// updateFor answers from the status cache without touching the disk.
func (s *libraryState) updateFor(gameID int) (bool, []string) {
	st, ok := s.statuses[gameID]
	if !ok {
		return false, nil
	}
	return st.HasUpdate, st.Diff
}

// gamesWithTasks are the games the download queue has something to say about.
// A change to the queue can only have changed the status of those, so the rest
// of the catalogue is left as it is rather than scanned again.
func gamesWithTasks(dm *DownloadManager, games []db.Game) []db.Game {
	dm.mu.RLock()
	all, _ := dm.Tasks.Get()
	dm.mu.RUnlock()

	queued := make(map[int]struct{}, len(all))
	for _, raw := range all {
		if task, ok := raw.(*DownloadTask); ok {
			queued[task.ID] = struct{}{}
		}
	}

	touched := make([]db.Game, 0, len(queued))
	for _, game := range games {
		if _, ok := queued[game.ID]; ok {
			touched = append(touched, game)
		}
	}
	return touched
}

// gamesWithUpdates returns the games whose cached status says an update is
// waiting for them.
func (s *libraryState) gamesWithUpdates(games []db.Game) []db.Game {
	pending := make([]db.Game, 0)
	for _, game := range games {
		if hasUpdate, _ := s.updateFor(game.ID); hasUpdate {
			pending = append(pending, game)
		}
	}
	return pending
}

// downloaded answers from the status cache without touching the disk.
func (s *libraryState) downloaded(gameID int) bool {
	return s.statuses[gameID].Downloaded
}

// libraryState is everything the library knows about its games beyond the
// catalogue rows: statuses, parsed facts, sizes, tags, and genres. One value
// per library, owned by the tab that built it, so a replaced library cannot
// bleed into the one on screen and tests cannot bleed into one another. It is
// read and written on the main thread only.
type libraryState struct {
	statuses  map[int]updateStatus
	statusURI fyne.URI
	facts     map[int]gameFacts
	sizes     map[sizeCacheKey]int64
	// catalogueSizes is a game's size for filtering: the largest single
	// platform it installs to, in any language, extras and DLCs included. It
	// does not depend on the download-form settings, so a "large games"
	// filter answers the same whichever platform the user has chosen.
	catalogueSizes map[int]int64
	tags           map[int][]string
	genres         map[int][]string
}

func newLibraryState() *libraryState {
	return &libraryState{
		statuses:       make(map[int]updateStatus),
		facts:          make(map[int]gameFacts),
		sizes:          make(map[sizeCacheKey]int64),
		catalogueSizes: make(map[int]int64),
		tags:           make(map[int][]string),
		genres:         make(map[int][]string),
	}
}

// initStatusPersistence points the state at the file statuses survive in
// between runs, and reads what an earlier session left there.
func (s *libraryState) initStatusPersistence() {
	if s.statusURI != nil {
		return
	}
	root := fyne.CurrentApp().Storage().RootURI()
	uri, err := storage.Child(root, "update_status_cache.json")
	if err == nil {
		s.statusURI = uri
		s.loadPersistedStatuses()
	}
}

func (s *libraryState) loadPersistedStatuses() {
	if s.statusURI == nil {
		return
	}
	reader, err := storage.Reader(s.statusURI)
	if err != nil {
		return
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil || len(data) == 0 {
		return
	}
	var raw map[string]updateStatus
	if json.Unmarshal(data, &raw) != nil {
		return
	}
	for k, v := range raw {
		if id, convErr := strconv.Atoi(k); convErr == nil {
			s.statuses[id] = v
		}
	}
}

func (s *libraryState) persistStatuses() {
	if s.statusURI == nil {
		return
	}
	writer, err := storage.Writer(s.statusURI)
	if err != nil {
		return
	}
	defer writer.Close()
	out := make(map[string]updateStatus, len(s.statuses))
	for id, st := range s.statuses {
		// Limit diff length persisted
		if len(st.Diff) > 50 {
			st.Diff = st.Diff[:50]
		}
		out[strconv.Itoa(id)] = st
	}
	enc := json.NewEncoder(writer)
	_ = enc.Encode(out)
}

func (s *libraryState) clearStatuses() {
	s.statuses = make(map[int]updateStatus)
	s.persistStatuses()
}
