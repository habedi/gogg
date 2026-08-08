package gui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/search"
	"github.com/rs/zerolog/log"
)

// searchBox is the search entry, which also clears itself on Escape: the
// fastest way out of a filter is the key that means "never mind".
type searchBox struct {
	widget.Entry
}

// What the toolbar's icon buttons say while the pointer rests on them. Named
// once so the tests can find a button by what it tells the user.
const (
	tipCollections = "Show or hide the collections"
	tipRefresh     = "Refresh the catalogue from GOG"
	tipShowList    = "Show as a list"
	tipShowCovers  = "Show as covers"
	tipMore        = "Sort and export"
	tipEstimate    = "Estimate the download size"
)

func newSearchBox() *searchBox {
	box := &searchBox{}
	box.ExtendBaseWidget(box)
	return box
}

func (b *searchBox) TypedKey(event *fyne.KeyEvent) {
	if event.Name == fyne.KeyEscape {
		b.SetText("")
		return
	}
	b.Entry.TypedKey(event)
}

// libraryTab holds all the components of the library tab UI.
type libraryTab struct {
	content     fyne.CanvasObject
	searchEntry *searchBox
	// selected is the game shown in the details pane, driven by the list.
	selected binding.Untyped
	// split is the divider between the list and the details, remembered
	// between runs.
	split *container.Split
	// refresh re-syncs the catalogue, the same as the Refresh button.
	refresh func()
	// storeHeader is the description of the selected game.
	storeHeader *fyne.Container
	// gallery is the artwork and store pictures of the selected game.
	gallery *gameGallery
	// sidebar lists the collections beside the games, when it is shown.
	sidebar *librarySidebar
	// showCollections is the toolbar button that shows and hides them.
	showCollections *iconButton
	// moreMenu builds what waits behind the toolbar's last button: the sort
	// order and the exports.
	moreMenu func() *fyne.Menu
	// listed is what the search and the filters have left showing, and relist
	// asks that question again after something they depend on has changed.
	listed func() []db.Game
	relist func()
	// pane is the right-hand side of the library, and dm the downloads it
	// starts.
	pane *detailsPane
	dm   *DownloadManager
	// close detaches what the library listens to. The catalogue signal belongs
	// to the whole app, so a library that has been replaced has to stop
	// following it or it goes on working for a window that is gone.
	close func()
}

// isGameDownloaded checks if a game has been successfully downloaded based on download history
func isGameDownloaded(dm *DownloadManager, gameID int) bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	allTasks, err := dm.Tasks.Get()
	if err != nil {
		return false
	}

	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if task.ID == gameID && task.State() == StateCompleted {
			return true
		}
	}
	return false
}

// getLastCompletedDownloadDir returns the download directory for the most recent completed download of a game, if any.
func getLastCompletedDownloadDir(dm *DownloadManager, gameID int) (string, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	allTasks, err := dm.Tasks.Get()
	if err != nil {
		return "", false
	}

	var (
		latestPath string
	)

	// Fallback: use time.Time from InstanceID, but we cannot compare with fyne.Time; instead compare via UnixNano
	var latestUnix int64 = -1
	for _, taskRaw := range allTasks {
		task := taskRaw.(*DownloadTask)
		if task.ID != gameID || task.State() != StateCompleted {
			continue
		}
		if t := task.InstanceID.UnixNano(); t > latestUnix {
			latestUnix = t
			latestPath = task.DownloadPath
		}
	}
	if latestUnix < 0 || latestPath == "" {
		return "", false
	}
	return latestPath, true
}

// readDownloadedMetadata loads the metadata.json stored alongside a completed download, if present.
func readDownloadedMetadata(dm *DownloadManager, gameID int) (*client.Game, error) {
	path, ok := getLastCompletedDownloadDir(dm, gameID)
	if !ok {
		return nil, fmt.Errorf("no completed download found")
	}
	metaPath := path + string(os.PathSeparator) + "metadata.json"
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, err
	}
	var g client.Game
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}
	return &g, nil
}

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

// searchDebounce is how long typing rests before the list is filtered again.
// Filtering runs a query over the whole catalogue, and running it between two
// keystrokes answers a question the user is still asking. Zero filters at
// once, which the tests rely on.
var searchDebounce = 200 * time.Millisecond

// debounced hands back an OnChanged handler that runs fn on the main thread
// once the changes have rested for delay. Each change restarts the clock. A
// delay of zero runs fn at once, on the caller.
func debounced(delay time.Duration, fn func()) func(string) {
	var timer *time.Timer
	return func(string) {
		if delay <= 0 {
			fn()
			return
		}
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(delay, func() { runOnMain(fn) })
	}
}

// computeUpdateStatus works the statuses out and records them, for callers that
// need the answer before they go on.
func computeUpdateStatus(dm *DownloadManager, games []db.Game) {
	applyStatuses(statusesFor(dm, games))
}

// applyStatuses records what was found. The window reads these, so they are
// only ever written on the thread that draws it. An update carries the moment
// it was first noticed: the same one seen again keeps its date, a different
// one gets today's.
func applyStatuses(found gameStatuses) {
	for id, status := range found {
		if status.HasUpdate {
			previous := updateStatusCache[id]
			if previous.HasUpdate && !previous.ChangedAt.IsZero() && slices.Equal(previous.Diff, status.Diff) {
				status.ChangedAt = previous.ChangedAt
			} else {
				status.ChangedAt = time.Now()
			}
		}
		updateStatusCache[id] = status
	}
	persistUpdateStatusCache()
}

// statusesFor works out what has been downloaded and what has changed since.
// It touches nothing the window reads, so it can run on a thread of its own.
func statusesFor(dm *DownloadManager, games []db.Game) gameStatuses {
	prefs := fyne.CurrentApp().Preferences()
	includeExtrasUpdates := prefs.BoolWithFallback("downloadForm.includeExtrasUpdates", false)
	includeDLCUpdates := prefs.BoolWithFallback("downloadForm.includeDLCUpdates", false)
	scanDirs := prefs.BoolWithFallback("downloadForm.scanDirsForDownloads", true)
	includePatchUpdates := prefs.BoolWithFallback("downloadForm.includePatchUpdates", false)
	langPref := prefs.StringWithFallback("downloadForm.language", "en")
	platformPref := prefs.StringWithFallback("downloadForm.platform", "windows")

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

// hasGameUpdateCached answers from the status cache without touching the disk.
func hasGameUpdateCached(gameID int) (bool, []string) {
	st, ok := updateStatusCache[gameID]
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
func gamesWithUpdates(games []db.Game) []db.Game {
	pending := make([]db.Game, 0)
	for _, game := range games {
		if hasUpdate, _ := hasGameUpdateCached(game.ID); hasUpdate {
			pending = append(pending, game)
		}
	}
	return pending
}

// isGameDownloadedCached answers from the status cache without touching the disk.
func isGameDownloadedCached(gameID int) bool {
	st, ok := updateStatusCache[gameID]
	if !ok {
		return false
	}
	return st.Downloaded
}

var updateStatusCache = make(map[int]updateStatus)
var updateStatusFileURI fyne.URI

func initUpdateStatusPersistence() {
	if updateStatusFileURI != nil {
		return
	}
	root := fyne.CurrentApp().Storage().RootURI()
	uri, err := storage.Child(root, "update_status_cache.json")
	if err == nil {
		updateStatusFileURI = uri
		loadPersistedUpdateStatus()
	}
}

func loadPersistedUpdateStatus() {
	if updateStatusFileURI == nil {
		return
	}
	reader, err := storage.Reader(updateStatusFileURI)
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
			updateStatusCache[id] = v
		}
	}
}

func persistUpdateStatusCache() {
	if updateStatusFileURI == nil {
		return
	}
	writer, err := storage.Writer(updateStatusFileURI)
	if err != nil {
		return
	}
	defer writer.Close()
	out := make(map[string]updateStatus, len(updateStatusCache))
	for id, st := range updateStatusCache {
		// Limit diff length persisted
		if len(st.Diff) > 50 {
			st.Diff = st.Diff[:50]
		}
		out[strconv.Itoa(id)] = st
	}
	enc := json.NewEncoder(writer)
	_ = enc.Encode(out)
}

func clearPersistedUpdateStatus() {
	updateStatusCache = make(map[int]updateStatus)
	persistUpdateStatusCache()
}

// parseGameData reads a game's stored data. It is a variable so tests can count
// how often the catalogue is read.
var parseGameData = client.ParseGameData

// gameFacts is what a query asks about a game that costs something to answer:
// reading its stored data. The data it was read from is kept alongside, so a
// game whose data has changed is read again rather than answered from what it
// used to say.
type gameFacts struct {
	data      string
	platforms []string
	languages []string
}

// parsedFacts keeps what each game's data said. A query is asked of every game
// on every keystroke, and reading the whole catalogue for each letter typed is
// what made searching a large library crawl.
var parsedFacts = map[int]gameFacts{}

// factsOf reads a game's stored data, or remembers what it said last time.
func factsOf(game db.Game) gameFacts {
	if facts, ok := parsedFacts[game.ID]; ok && facts.data == game.Data {
		return facts
	}

	facts := gameFacts{data: game.Data}
	if parsed, err := parseGameData(game.Data); err == nil {
		for _, platform := range offeredPlatforms(parsed) {
			facts.platforms = append(facts.platforms, strings.ToLower(platform))
		}
		facts.languages = languageNamesAndCodes(offeredLanguages(parsed))
	}
	parsedFacts[game.ID] = facts
	return facts
}

// forgetParsedGames drops what was read of the catalogue, because it is no
// longer the same catalogue.
func forgetParsedGames() {
	parsedFacts = make(map[int]gameFacts)
	sizeCache = make(map[sizeCacheKey]int64)
}

// sizeCacheKey identifies an estimate together with the settings it was
// computed under, so changing any of them yields a fresh estimate.
type sizeCacheKey struct {
	id             int
	lang, platform string
	extras, dlcs   bool
}

var sizeCache = make(map[sizeCacheKey]int64)

func estimateGameSize(game db.Game) int64 {
	prefs := fyne.CurrentApp().Preferences()
	key := sizeCacheKey{
		id:       game.ID,
		lang:     prefs.StringWithFallback("downloadForm.language", "en"),
		platform: prefs.StringWithFallback("downloadForm.platform", "windows"),
		extras:   prefs.BoolWithFallback("downloadForm.extras", true),
		dlcs:     prefs.BoolWithFallback("downloadForm.dlcs", true),
	}
	if v, ok := sizeCache[key]; ok {
		return v
	}
	parsed, err := parseGameData(game.Data)
	if err != nil {
		sizeCache[key] = 0
		return 0
	}
	// The stored game data names languages in full, not by code.
	langFullName, ok := client.GameLanguages[key.lang]
	if !ok {
		sizeCache[key] = 0
		return 0
	}
	sz, err := parsed.EstimateStorageSize(langFullName, key.platform, key.extras, key.dlcs)
	if err != nil {
		sizeCache[key] = 0
		return 0
	}
	sizeCache[key] = sz
	return sz
}

// gameTags is what the user has marked games with, read once per refresh
// because a query is asked of every game on every keystroke.
var gameTags = map[int][]string{}

// loadGameTags reads what the user has marked games with. It is read in one go
// and kept, because a query is asked of every game on every keystroke.
func loadGameTags() {
	tags, err := db.AllTags(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Could not read game tags")
		return
	}
	gameTags = tags
}

// gameGenres is what GOG's store says each game is, for games whose store
// pages have been looked up. Read in one go and kept, like the tags.
var gameGenres = map[int][]string{}

// loadGameGenres reads the genres out of the stored lookups.
func loadGameGenres() {
	records, err := db.AllGameMetadata(context.Background())
	if err != nil {
		log.Debug().Err(err).Msg("Could not read stored metadata")
		return
	}

	genres := make(map[int][]string, len(records))
	for _, record := range records {
		var meta client.GameMetadata
		if json.Unmarshal(record.Data, &meta) != nil || len(meta.Genres) == 0 {
			continue
		}
		genres[record.GameID] = meta.Genres
	}
	gameGenres = genres
}

// gameHasTag says whether a game carries a tag, read from the cache the
// searches read.
func gameHasTag(gameID int, tag string) bool {
	for _, held := range gameTags[gameID] {
		if held == tag {
			return true
		}
	}
	return false
}

// anyChoice is what a select says when it is not filtering on anything.
const anyChoice = "Any"

// filterChoices is what the filter dialog was set to.
type filterChoices struct {
	Downloaded, HasUpdate bool
	MinSize, MaxSize      string
	Platform, Language    string
	Tag, Genre            string
	// UpdatedSince narrows to updates noticed within an age or since a date.
	UpdatedSince string
}

// filterTerms turns what the filter dialog was set to into the terms it writes
// into the search box, so the dialog and the box stay the same filter.
func filterTerms(choices filterChoices) []string {
	var terms []string
	if choices.Downloaded {
		terms = append(terms, "downloaded:yes")
	}
	if choices.HasUpdate {
		terms = append(terms, "updates:yes")
	}
	if size := strings.ReplaceAll(strings.TrimSpace(choices.MinSize), " ", ""); size != "" {
		terms = append(terms, "size:>="+size)
	}
	if size := strings.ReplaceAll(strings.TrimSpace(choices.MaxSize), " ", ""); size != "" {
		terms = append(terms, "size:<="+size)
	}
	for _, field := range []struct{ name, chosen string }{
		{"platform", choices.Platform}, {"lang", choices.Language},
		{"tag", choices.Tag}, {"genre", choices.Genre},
	} {
		if value := strings.TrimSpace(field.chosen); value != "" && value != anyChoice {
			terms = append(terms, field.name+":"+value)
		}
	}
	if since := strings.ReplaceAll(strings.TrimSpace(choices.UpdatedSince), " ", ""); since != "" {
		terms = append(terms, "updated:>="+since)
	}
	return terms
}

// withoutHidden leaves out the games the user has marked hidden, unless the
// query is about hidden games. The tag put them in a collection of their own
// and left them in every other list as well, which is not what hiding means.
func withoutHidden(query search.Query) search.Query {
	if query.Mentions("hidden") {
		return query
	}
	notHidden, err := search.Parse("hidden:no")
	if err != nil {
		return query
	}
	return query.And(notHidden)
}

// factsFor describes a game to a query. Working out the size or the platforms
// means parsing the stored data, so it is only done for queries that ask.
func factsFor(game db.Game, needs search.Needs) search.Facts {
	facts := search.Facts{Title: game.Title}
	if status, ok := updateStatusCache[game.ID]; ok {
		facts.Downloaded = status.Downloaded
		facts.HasUpdate = status.HasUpdate
		facts.UpdatedAt = status.ChangedAt
	}
	if needs.Tags {
		facts.Tags = gameTags[game.ID]
	}
	if needs.Genres {
		facts.Genres = gameGenres[game.ID]
	}
	if needs.Size {
		facts.SizeBytes = estimateGameSize(game)
	}
	if needs.Platforms || needs.Languages {
		read := factsOf(game)
		if needs.Platforms {
			facts.Platforms = read.platforms
		}
		if needs.Languages {
			facts.Languages = read.languages
		}
	}
	return facts
}

// languageNamesAndCodes lets a game be asked for either way: lang:german and
// lang:de mean the same thing to someone typing quickly.
func languageNamesAndCodes(names []string) []string {
	both := make([]string, 0, len(names)*2)
	for _, name := range names {
		both = append(both, name)
		for code, full := range client.GameLanguages {
			if strings.EqualFold(full, name) {
				both = append(both, code)
				break
			}
		}
	}
	return both
}

// newFiltersButton edits the field terms of the search, leaving the words the
// user typed. The dialog and the search box are then the same filter, one of
// them just easier to discover.
func newFiltersButton(searchEntry *widget.Entry, refresh func()) *widget.Button {
	var dlg *dialog.CustomDialog
	btn := widget.NewButtonWithIcon("Filters", theme.SearchIcon(), func() {
		current, _ := search.Parse(searchEntry.Text)

		downloadedChk := widget.NewCheck("Downloaded only", nil)
		downloadedChk.SetChecked(current.HasTerm("downloaded", "yes"))
		updateChk := widget.NewCheck("Has update", nil)
		updateChk.SetChecked(current.HasTerm("updates", "yes"))

		// The sizes are suggested the way the app writes them, so what the
		// dialog hints at is what a game's size beside it says.
		sizeMinEntry := widget.NewEntry()
		sizeMinEntry.SetPlaceHolder("10 GiB")
		sizeMinEntry.SetText(current.TermValue("size", ">="))
		sizeMaxEntry := widget.NewEntry()
		sizeMaxEntry.SetPlaceHolder("50 GiB")
		sizeMaxEntry.SetText(current.TermValue("size", "<="))

		// The box understands more than downloads and sizes, so the dialog
		// offers the rest of it rather than half.
		platformSelect := widget.NewSelect([]string{anyChoice, "windows", "mac", "linux"}, nil)
		platformSelect.SetSelected(chosenOr(current.TermValue("platform", ""), anyChoice))
		languageSelect := widget.NewSelect(append([]string{anyChoice}, languageCodesOffered()...), nil)
		languageSelect.SetSelected(chosenOr(current.TermValue("lang", ""), anyChoice))
		tagEntry := widget.NewEntry()
		tagEntry.SetPlaceHolder("finished")
		tagEntry.SetText(current.TermValue("tag", ""))
		genreEntry := widget.NewEntry()
		genreEntry.SetPlaceHolder("strategy")
		genreEntry.SetText(current.TermValue("genre", ""))
		updatedEntry := widget.NewEntry()
		updatedEntry.SetPlaceHolder("30d, or 2026-01-31")
		updatedEntry.SetText(current.TermValue("updated", ">="))

		apply := func(terms ...string) {
			query := strings.TrimSpace(strings.Join(append([]string{search.Words(searchEntry.Text)}, terms...), " "))
			searchEntry.SetText(query)
			refresh()
			dlg.Hide()
		}

		applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
			apply(filterTerms(filterChoices{
				Downloaded: downloadedChk.Checked, HasUpdate: updateChk.Checked,
				MinSize: sizeMinEntry.Text, MaxSize: sizeMaxEntry.Text,
				Platform: platformSelect.Selected, Language: languageSelect.Selected,
				Tag: tagEntry.Text, Genre: genreEntry.Text,
				UpdatedSince: updatedEntry.Text,
			})...)
		})
		resetBtn := widget.NewButtonWithIcon("Reset", theme.ViewRefreshIcon(), func() { apply() })

		content := container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Min Size", sizeMinEntry),
				widget.NewFormItem("Max Size", sizeMaxEntry),
				widget.NewFormItem("Platform", platformSelect),
				widget.NewFormItem("Language", languageSelect),
				widget.NewFormItem("Tag", tagEntry),
				widget.NewFormItem("Genre", genreEntry),
				widget.NewFormItem("Updated in", updatedEntry),
			),
			container.NewGridWithColumns(2, downloadedChk, updateChk),
			widget.NewLabel("These become terms in the search box, where they can also be typed."),
			container.NewHBox(applyBtn, resetBtn),
		)
		dlg = dialog.NewCustom("Library Filters", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
		dlg.Resize(fyne.NewSize(460, 420))
		dlg.Show()
	})
	return btn
}

// chosenOr is what a select shows: what the search says, or that it is not
// filtering on this at all.
func chosenOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// languageCodesOffered lists the language codes a search can ask for.
func languageCodesOffered() []string {
	codes := make([]string, 0, len(client.GameLanguages))
	for code := range client.GameLanguages {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

// LibraryTabUI builds the catalogue tab. onLogin is invoked when a signed-out
// user asks to log in.
func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager, onLogin func()) *libraryTab {
	token, _ := db.GetTokenRecord()
	if token == nil {
		loginBtn := widget.NewButtonWithIcon("Log In to GOG", theme.LoginIcon(), func() {
			if onLogin != nil {
				onLogin()
			}
		})
		loginBtn.Importance = widget.HighImportance
		content := emptyState(theme.WarningIcon(), "Not logged in",
			"Log in to GOG to see the games you own.", loginBtn)
		return &libraryTab{
			content:     content,
			searchEntry: newSearchBox(),
			selected:    binding.NewUntyped(),
			refresh:     func() {},
			gallery:     newGameGallery(nil, win),
			listed:      func() []db.Game { return nil },
			relist:      func() {},
			dm:          dm,
			close:       func() {},
		}
	}

	allGames, _ := db.GetCatalogue()
	loadGameTags()
	loadGameGenres()
	// Set when this library is replaced. Answers that were on their way to it
	// are dropped rather than delivered to a pane nothing shows anymore.
	var closed atomic.Bool
	gamesListBinding := binding.NewUntypedList()
	var sidebar *librarySidebar
	selectedGameBinding := binding.NewUntyped()
	isSortAscending := true
	// sortByPurchase puts the latest buys first, the order GOG's account
	// listing was read in; titles break the ties.
	sortByPurchase := false

	gameCountLabel := widget.NewLabel("")

	searchEntry := newSearchBox()
	// The box takes the same filters the collections are made of, and nothing
	// else on screen says so.
	searchEntry.SetPlaceHolder("Search titles, or filter: downloaded:no platform:linux")
	// A filter that cannot be read is dropped and the words alone are searched
	// for, so the box has to show that it is not doing what it says.
	searchEntry.Validator = func(text string) error {
		_, err := search.Parse(text)
		return err
	}
	clearSearchBtn := newIconButton(theme.CancelIcon(), "Clear the search", func() {
		searchEntry.SetText("")
	})
	searchEntry.ActionItem = clearSearchBtn
	clearSearchBtn.Hide()

	prefs := fyne.CurrentApp().Preferences()
	sel := newGameSelection()
	// These are assigned once every widget they touch exists.
	var afterSelectionChange func()
	var refreshUpdatesSummary func()

	updatesLabel := widget.NewLabel("")
	updateAllBtn := widget.NewButtonWithIcon("Update All", theme.DownloadIcon(), nil)
	updateAllBtn.Importance = widget.HighImportance
	updateAllBtn.Hide()

	covers := newCoverCache(coverCacheDir())
	metadata := newMetadataCache()

	var gameListWidget *activatableList
	var gameGridWidget *activatableGrid
	displayedGames := func() []db.Game {
		items, _ := gamesListBinding.Get()
		games := make([]db.Game, 0, len(items))
		for _, item := range items {
			games = append(games, item.(db.Game))
		}
		return games
	}

	// A search that matched nothing left a blank list, which reads as a library
	// that has not loaded rather than as an answer.
	noMatches := emptyState(theme.SearchIcon(), "No games match",
		"Nothing in the catalogue fits this search.",
		widget.NewButton("Clear Search", func() { searchEntry.SetText("") }))
	// showGames puts the right view in the list pane. It is assigned once the
	// widgets it switches between exist.
	var showGames func()

	// updateDisplayedGames decides which games are listed. Searching, sorting
	// and filtering do not change a game's status, so this does no I/O.
	updateDisplayedGames := func() {
		query, err := search.Parse(searchEntry.Text)
		if err != nil {
			// Half-typed filters are the normal state of a search box, so what
			// is there so far is treated as words rather than as a mistake.
			query, _ = search.Parse(search.Words(searchEntry.Text))
		}
		query = withoutHidden(query)
		needs := query.Needs()

		displayGames := make([]db.Game, 0, len(allGames))
		for _, game := range allGames {
			if !query.Match(factsFor(game, needs)) {
				continue
			}
			displayGames = append(displayGames, game)
		}

		sort.Slice(displayGames, func(i, j int) bool {
			if sortByPurchase {
				// Rank 1 is the most recent buy; games from catalogues
				// refreshed before gogg recorded ranks sink to the bottom.
				left, right := displayGames[i].PurchaseRank, displayGames[j].PurchaseRank
				if left != right {
					if left == 0 {
						return false
					}
					if right == 0 {
						return true
					}
					return left < right
				}
			}
			left := strings.ToLower(displayGames[i].Title)
			right := strings.ToLower(displayGames[j].Title)
			if isSortAscending {
				return left < right
			}
			return left > right
		})

		_ = gamesListBinding.Set(untypedSlice(displayGames))
		if gameGridWidget != nil {
			gameGridWidget.Refresh()
		}
		gameCountLabel.SetText(fmt.Sprintf("%d games found", len(displayGames)))
		if sidebar != nil && sidebar.content.Visible() {
			sidebar.syncTo(searchEntry.Text)
		}
		if strings.TrimSpace(searchEntry.Text) == "" {
			clearSearchBtn.Hide()
		} else {
			clearSearchBtn.Show()
		}
		if showGames != nil {
			showGames()
		}
	}

	// recomputeStatuses refreshes the cached download and update status for the
	// whole library. It reads the filesystem and reparses every stored game, so
	// it runs only when something that can change a status happened. onDone,
	// when given, runs once the statuses are known.
	recomputeStatuses := func(onDone func()) {
		games := allGames
		updatesLabel.SetText("Checking downloads...")
		statusWorker(
			func() gameStatuses { return statusesFor(dm, games) },
			func(found gameStatuses) {
				if closed.Load() {
					return
				}
				applyStatuses(found)
				if sidebar != nil && sidebar.content.Visible() {
					sidebar.refresh(allGames)
				}
				updateDisplayedGames()
				if refreshUpdatesSummary != nil {
					refreshUpdatesSummary()
				}
				if onDone != nil {
					onDone()
				}
			})
	}

	// Typing is followed at a small distance: each keystroke restarts the
	// clock, and the list is filtered once the typing rests.
	searchEntry.OnChanged = debounced(searchDebounce, updateDisplayedGames)

	displayedForGrid := func() []db.Game { return displayedGames() }

	gameGridWidget = newActivatableGrid(
		func() int { return len(displayedForGrid()) },
		newGameCell,
		func(id widget.GridWrapItemID, obj fyne.CanvasObject) {
			games := displayedForGrid()
			if id >= len(games) {
				return
			}
			cell, ok := obj.(*gameCell)
			if !ok {
				return
			}
			bindGameCell(cell, games[id], sel, covers, dm, func() { afterSelectionChange() })
		},
	)

	listContent := container.NewStack()
	gameListWidget = newActivatableList(gamesListBinding,
		newGameRow,
		func(item binding.DataItem, obj fyne.CanvasObject) {
			gameRaw, _ := item.(binding.Untyped).Get()
			game, ok := gameRaw.(db.Game)
			if !ok {
				return
			}
			bindGameRow(obj, game, sel, covers, dm, func() { afterSelectionChange() })
		},
	)
	gameGridWidget.OnSelected = func(id widget.GridWrapItemID) {
		games := displayedForGrid()
		if id >= len(games) {
			return
		}
		_ = selectedGameBinding.Set(games[id])
	}
	gameGridWidget.OnUnselected = func(widget.GridWrapItemID) {
		_ = selectedGameBinding.Set(nil)
	}

	gameListWidget.OnSelected = func(id widget.ListItemID) {
		gameRaw, _ := gamesListBinding.GetValue(id)
		_ = selectedGameBinding.Set(gameRaw)
	}
	gameListWidget.OnUnselected = func(id widget.ListItemID) {
		_ = selectedGameBinding.Set(nil)
	}

	// A download finishing changes what a game is, so everything that says what
	// a game is has to follow it: the collections count the statuses, the list
	// may be filtered by them, and the rows carry the badges. Working the
	// statuses out reads the filesystem, so it goes to the worker rather than
	// holding up the thread this listener fires on.
	followDownloads := binding.NewDataListener(func() {
		if closed.Load() {
			return
		}
		touched := gamesWithTasks(dm, allGames)
		statusWorker(
			func() gameStatuses { return statusesFor(dm, touched) },
			func(found gameStatuses) {
				if closed.Load() {
					return
				}
				applyStatuses(found)
				if sidebar != nil && sidebar.content.Visible() {
					sidebar.refresh(allGames)
				}
				updateDisplayedGames()
				if refreshUpdatesSummary != nil {
					refreshUpdatesSummary()
				}
				gameListWidget.Refresh()
			})
	})
	// The list of downloads changes when one is added; a download that finishes
	// changes only its state. The badges follow both.
	dm.Tasks.AddListener(followDownloads)
	dm.states().AddListener(followDownloads)

	// Covers are the way a person recognises their games, so they are the first
	// thing a new library shows. Whoever chose the list keeps it.
	showingGrid := prefs.BoolWithFallback(prefGridView, true)
	var viewBtn *iconButton
	showGames = func() {
		if len(allGames) == 0 {
			return
		}
		switch {
		case len(displayedGames()) == 0:
			listContent.Objects = []fyne.CanvasObject{noMatches}
		case showingGrid:
			listContent.Objects = []fyne.CanvasObject{gameGridWidget}
		default:
			listContent.Objects = []fyne.CanvasObject{gameListWidget}
		}
		// The button offers the view it would switch to.
		if showingGrid {
			viewBtn.SetIcon(theme.ListIcon())
			viewBtn.tip = tipShowList
		} else {
			viewBtn.SetIcon(theme.GridIcon())
			viewBtn.tip = tipShowCovers
		}
		listContent.Refresh()
	}
	viewBtn = newIconButton(theme.ListIcon(), tipShowList, func() {
		showingGrid = !showingGrid
		prefs.SetBool(prefGridView, showingGrid)
		showGames()
	})

	// Both the toolbar and the empty-library placeholder offer a refresh, and
	// they start the same job, so pressing either has to close both while it
	// runs. Only one of them exists at a time, hence the checks for nil.
	var refreshBtn *iconButton
	var refreshNowBtn *widget.Button
	setRefreshEnabled := func(enabled bool) {
		buttons := make([]fyne.Disableable, 0, 2)
		if refreshBtn != nil {
			buttons = append(buttons, refreshBtn)
		}
		if refreshNowBtn != nil {
			buttons = append(buttons, refreshNowBtn)
		}
		for _, button := range buttons {
			if enabled {
				button.Enable()
				continue
			}
			button.Disable()
		}
	}
	// The whole library's store details fill in quietly in the background, so
	// genres and descriptions work for every game, not only the clicked ones.
	startSweep := func() {
		if !prefs.BoolWithFallback(prefMetadataSweep, true) || len(allGames) == 0 {
			return
		}
		metadata.sweep(allGames, func() {
			if closed.Load() {
				return
			}
			if sidebar != nil && sidebar.content.Visible() {
				sidebar.refresh(allGames)
			}
			updateDisplayedGames()
		})
	}

	// A refresh that brought updates is news, so it is announced; one that
	// brought none already shows in the counts, and a dialog saying "nothing"
	// after every refresh would teach people to dismiss dialogs unread.
	announceRefreshOutcome := func() {
		pending := gamesWithUpdates(allGames)
		if len(pending) == 0 {
			return
		}
		titles := make([]string, 0, len(pending))
		for _, game := range pending {
			titles = append(titles, game.Title)
		}
		verb := "have"
		if len(pending) == 1 {
			verb = "has"
		}
		dialog.ShowInformation("Updates waiting",
			fmt.Sprintf("%d %s %s new files since being downloaded: %s",
				len(pending), gamesWord(len(pending)), verb, joinTitles(titles)), win)
	}
	onFinishRefresh := func() {
		allGames, _ = db.GetCatalogue()
		loadGameTags()
		loadGameGenres()
		forgetParsedGames()
		recomputeStatuses(announceRefreshOutcome)
		setRefreshEnabled(true)
		showGames()
		// A refresh may have brought games nobody has looked up yet.
		startSweep()
	}
	startRefresh := func() {
		setRefreshEnabled(false)
		refreshCatalogue(win, authService, onFinishRefresh)
	}

	if len(allGames) == 0 {
		refreshNowBtn = widget.NewButton("Refresh Catalogue", startRefresh)
		listContent.Add(emptyState(theme.InfoIcon(), "Nothing in the catalogue yet",
			"Refresh to fetch the games you own from GOG.", refreshNowBtn))
	} else {
		listContent.Add(gameListWidget)
		// Filled before showGames looks, or an unfiltered library reads as a
		// search that matched nothing.
		updateDisplayedGames()
	}
	// Load any status cached by an earlier session before recomputing, so stale
	// entries cannot overwrite fresh ones.
	initUpdateStatusPersistence()
	recomputeStatuses(nil)
	startSweep()

	// The search box is left as it is: emptying it threw away the filter, or the
	// collection, the user was looking at.
	refreshBtn = newIconButton(theme.ViewRefreshIcon(), tipRefresh, startRefresh)

	// Named for what choosing it does, like the view button beside it: one
	// entry naming the state and its neighbour naming the action reads as a
	// contradiction.
	sortLabel := func() string {
		if isSortAscending {
			return "Sort Z-A"
		}
		return "Sort A-Z"
	}
	// Sorting and exporting are reached for rarely, so they wait in a menu
	// rather than widening the toolbar for everyone. The menu is built afresh
	// each time it opens, so the sort entry names the order it would switch to.
	// The purchase entry names the order it would switch to, like the title
	// one beside it.
	purchaseLabel := func() string {
		if sortByPurchase {
			return "Sort by Title"
		}
		return "Sort by Purchase Date"
	}
	moreMenu := func() *fyne.Menu {
		return fyne.NewMenu("",
			fyne.NewMenuItem(sortLabel(), func() {
				sortByPurchase = false
				isSortAscending = !isSortAscending
				updateDisplayedGames()
				gameListWidget.Refresh()
			}),
			fyne.NewMenuItem(purchaseLabel(), func() {
				sortByPurchase = !sortByPurchase
				updateDisplayedGames()
				gameListWidget.Refresh()
			}),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("Export Game List as CSV", func() { ExportCatalogueAction(win, "csv") }),
			fyne.NewMenuItem("Export Full Catalogue as JSON", func() { ExportCatalogueAction(win, "json") }),
		)
	}
	var moreBtn *iconButton
	moreBtn = newIconButton(theme.MoreHorizontalIcon(), tipMore, func() {
		popup := widget.NewPopUpMenu(moreMenu(), win.Canvas())
		// Dropped from the button itself: where a button sits inside its own
		// container is not where it is on the canvas, and taking the one for the
		// other put this menu at the top of the window.
		popup.ShowAtRelativePosition(fyne.NewPos(0, moreBtn.Size().Height), moreBtn)
	})

	// The button is made here so the toolbar can hold it; what it does is wired
	// once the collections it shows exist.
	collectionsBtn := newIconButton(theme.MenuIcon(), tipCollections, nil)
	filtersBtn := newFiltersButton(&searchEntry.Entry, updateDisplayedGames)
	// The buttons scroll rather than forcing a minimum width on the window; the
	// update summary stays pinned to the right.
	toolbarButtons := container.NewHScroll(
		container.NewHBox(collectionsBtn, refreshBtn, viewBtn,
			filtersBtn, updateAllBtn, moreBtn))
	toolbar := container.NewBorder(nil, nil, nil,
		container.NewHBox(updatesLabel, gameCountLabel), toolbarButtons)
	selectionLabel := widget.NewLabel("")
	// Changing many rows at once needs the list redrawn; a row the user ticked
	// themselves already shows the right state.
	applyBulkSelection := func(change func()) {
		change()
		afterSelectionChange()
		gameListWidget.Refresh()
		gameGridWidget.Refresh()
	}
	selectAllBtn := widget.NewButton("Select All Shown", func() {
		applyBulkSelection(func() { sel.selectAll(displayedGames()) })
	})
	clearSelectionBtn := widget.NewButton("Clear Selection", func() {
		applyBulkSelection(sel.clear)
	})
	selectionControls := container.NewHBox(selectAllBtn, clearSelectionBtn,
		layout.NewSpacer(), selectionLabel)

	leftTopContainer := container.NewVBox(searchEntry, selectionControls, widget.NewSeparator())
	listPane := container.NewBorder(leftTopContainer, toolbar, nil, nil, listContent)

	// A collection is a stored query, so picking one is the same as typing it,
	// keeping whatever words are already in the box.
	sidebar = newLibrarySidebar(libraryCollections(), func(query string) {
		text := strings.TrimSpace(search.Words(searchEntry.Text) + " " + query)
		searchEntry.SetText(text)
		updateDisplayedGames()
	})
	leftPane := container.NewBorder(nil, nil, sidebar.content, nil, listPane)

	// Counting the collections parses the catalogue, so it is only done while
	// they are on screen.
	showCollections := func(shown bool) {
		prefs.SetBool(prefSidebar, shown)
		if shown {
			sidebar.refresh(allGames)
			sidebar.syncTo(searchEntry.Text)
			sidebar.content.Show()
		} else {
			sidebar.content.Hide()
		}
		// Hiding a child sets a flag on the child; the pane holding it has to be
		// told to lay itself out again, or the space stays where it was.
		leftPane.Refresh()
	}
	collectionsBtn.OnTapped = func() { showCollections(!sidebar.content.Visible()) }
	showCollections(prefs.BoolWithFallback(prefSidebar, false))

	detailsBox := container.NewVBox()
	pane := createDetailsPane(win, authService, dm, selectedGameBinding,
		sel, func() []db.Game { return allGames }, detailsBox, covers,
		func() {
			// A mark moves games between collections and may take the game
			// off the list, so both follow it.
			if sidebar != nil && sidebar.content.Visible() {
				sidebar.refresh(allGames)
			}
			updateDisplayedGames()
		})
	form := pane.form

	// Enter on the focused list or grid downloads what is selected: the last
	// step of a flow the arrow keys and space already carry.
	gameListWidget.onActivate = func() { form.download() }
	gameGridWidget.onActivate = func() { form.download() }

	afterSelectionChange = func() {
		if n := sel.count(); n > 0 {
			selectionLabel.SetText(fmt.Sprintf("%d selected", n))
		} else {
			selectionLabel.SetText("")
		}
		form.relabel()
	}
	afterSelectionChange()

	refreshUpdatesSummary = func() {
		pending := gamesWithUpdates(allGames)
		if len(pending) == 0 {
			updatesLabel.SetText("")
			updateAllBtn.Hide()
			return
		}
		updatesLabel.SetText(fmt.Sprintf("%d %s with updates", len(pending), gamesWord(len(pending))))
		updateAllBtn.SetText(fmt.Sprintf("Update All (%d)", len(pending)))
		updateAllBtn.Show()
	}
	updateAllBtn.OnTapped = func() {
		pending := gamesWithUpdates(allGames)
		if len(pending) == 0 {
			return
		}
		dialog.ShowConfirm("Update All",
			fmt.Sprintf("Download updates for %d %s?", len(pending), gamesWord(len(pending))),
			func(confirmed bool) {
				if !confirmed {
					return
				}
				result, err := form.queue(pending)
				if err != nil {
					showErrorDialog(win, "Cannot start downloads", err)
					return
				}
				dialog.ShowInformation("Downloads", result.summary(), win)
			}, win)
	}
	refreshUpdatesSummary()
	rightPane := pane.content
	pane.body.Hide()
	pane.empty.Show()

	selectedGameBinding.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGameBinding.Get()
		if gameRaw == nil {
			fillStoreHeader(pane, nil)
			pane.storeStatus.Hide()
			pane.favorite.Hide()
			pane.hide.Hide()
			pane.gallery.show(nil, 0)
			pane.body.Hide()
			pane.empty.Show()
			form.narrowTo(db.Game{})
			pane.title.SetText("")
			return
		}
		game := gameRaw.(db.Game)
		pane.empty.Hide()
		pane.title.SetText(game.Title)
		pane.refreshMarks(game)
		pane.favorite.Show()
		pane.hide.Show()
		// The facts gogg already holds show at once; what GOG's store adds
		// arrives when it arrives.
		fillDetails(pane, game, dm, nil)
		fillStoreHeader(pane, nil)
		showGallery(pane.gallery, game, nil)
		pane.storeStatus.SetText("Fetching store details...")
		pane.storeStatus.Show()
		stillShowing := func() bool {
			if closed.Load() {
				return false
			}
			current, _ := selectedGameBinding.Get()
			shown, ok := current.(db.Game)
			return ok && shown.ID == game.ID
		}
		metadata.load(game.ID, func(int) bool { return stillShowing() }, func(meta client.GameMetadata) {
			pane.storeStatus.Hide()
			fillDetails(pane, game, dm, &meta)
			fillStoreHeader(pane, &meta)
			showGallery(pane.gallery, game, meta.Screenshots)
		}, func() {
			pane.storeStatus.SetText("No store details for this game.")
		})

		form.narrowTo(game)
		pane.body.Show()
	}))
	// AddListener invokes the listener once on registration, and the status
	// worked out above is still valid at that point.
	catalogueJustRegistered := true
	catalogueListener := binding.NewDataListener(func() {
		if catalogueJustRegistered {
			catalogueJustRegistered = false
			return
		}
		clearPersistedUpdateStatus()
		forgetParsedGames()
		recomputeStatuses(nil)
	})
	catalogueUpdated.AddListener(catalogueListener)

	// The rules for spotting an update are set in Settings, and what was worked
	// out under the old ones is no longer the answer.
	settingsJustRegistered := true
	settingsListener := binding.NewDataListener(func() {
		if settingsJustRegistered {
			settingsJustRegistered = false
			return
		}
		recomputeStatuses(nil)
	})
	updateSettingsChanged.AddListener(settingsListener)
	split := container.NewHSplit(leftPane, rightPane)
	split.Offset = loadWindowState(prefs).SplitOffset

	return &libraryTab{
		content:         split,
		searchEntry:     searchEntry,
		selected:        selectedGameBinding,
		split:           split,
		refresh:         func() { refreshBtn.OnTapped() },
		storeHeader:     pane.storeHeader,
		gallery:         pane.gallery,
		listed:          displayedGames,
		sidebar:         sidebar,
		showCollections: collectionsBtn,
		moreMenu:        moreMenu,
		relist:          updateDisplayedGames,
		pane:            pane,
		dm:              dm,
		close: func() {
			closed.Store(true)
			metadata.close()
			catalogueUpdated.RemoveListener(catalogueListener)
			updateSettingsChanged.RemoveListener(settingsListener)
		},
	}
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

// rememberedDownloadPath is where a download would go without being told. What
// the user last typed comes first, because it is the box they typed it into; a
// catalogue from a version that only recorded where downloads went still opens
// on that.
func rememberedDownloadPath(prefs fyne.Preferences) string {
	if path := prefs.String("downloadForm.path"); path != "" {
		return path
	}
	return prefs.StringWithFallback("lastUsedDownloadPath", "")
}

// downloadForm exposes what the rest of the library needs from the options pane.
type downloadForm struct {
	// options is what a download is made of: the path, the choices and the
	// switches. It goes in a tab of its own.
	options fyne.CanvasObject
	// actions are the buttons, which stay in sight at the foot of the pane.
	actions fyne.CanvasObject
	// relabel brings the download button in line with the current selection.
	relabel func()
	// queue starts downloads for the given games with the options on screen.
	queue func(games []db.Game) (batchResult, error)
	// download is what the download button does, reachable from the keyboard.
	download func()
	// narrowTo restricts the language and platform choices to what a game
	// offers; a zero game restores the full lists.
	narrowTo func(game db.Game)
	// showStore points the GOG button at a game's store page, hiding it for
	// games GOG no longer lists.
	showStore func(url string)
}

// artworkSize is the least room the details pane gives a game's picture. It
// grows with the pane from there, so this is a floor rather than a size: the
// smaller it is, the narrower gogg's window is allowed to be.
var artworkSize = fyne.NewSize(320, 180)

// detailsPane is the right-hand side of the library: the artwork, the facts and
// the download options, in that order.
type detailsPane struct {
	content fyne.CanvasObject
	form    *downloadForm
	gallery *gameGallery
	// title names the game the pane is describing.
	title *CopyableLabel
	// storeHeader is the description, shown on the overview.
	storeHeader *fyne.Container
	// storeStatus says where the description is: being fetched, or not to be
	// had. It is hidden once the description itself can speak.
	storeStatus *widget.Label
	// facts is everything gogg knows about the game.
	facts *fyne.Container
	// favorite and hide mark the shown game with the tags the collections
	// collect; refreshMarks brings their icons in line with the game's tags.
	favorite, hide *iconButton
	refreshMarks   func(db.Game)
	// body holds everything about a game, and is hidden when none is selected.
	body *fyne.Container
	// empty takes its place then, saying what would fill the pane.
	empty fyne.CanvasObject
	// tabs are what a game is: the overview it is downloaded from, and the
	// facts. The buttons are not in either, so they do not scroll away.
	tabs *container.AppTabs
}

// createDetailsPane builds the pane. detailsBox is filled with the selected
// game's facts by the caller. The artwork leads, because a picture says which
// game this is at a glance, and the options sit under it so that choosing them
// and pressing Download happen in the same place.
func createDetailsPane(win fyne.Window, authService *auth.Service, dm *DownloadManager,
	selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
	detailsBox *fyne.Container, covers *coverCache, onTagsChanged func(),
) *detailsPane {
	form := createDownloadForm(win, authService, dm, selectedGame, sel, catalogue)

	title := NewCopyableLabel("Select a game from the list")
	title.TextStyle = fyne.TextStyle{Bold: true}
	// The title is the headline of the pane, so it is set in headline type.
	title.SizeName = theme.SizeNameSubHeadingText
	title.Truncation = fyne.TextTruncateEllipsis

	// The marks a game can carry, beside its name where they read as facts
	// about it: a favorite star, and the way to hide it from the list.
	favBtn := newIconButton(iconStarOutline, "Add to favorites", nil)
	hideBtn := newIconButton(theme.VisibilityOffIcon(), "Hide this game", nil)
	refreshMarks := func(game db.Game) {
		if gameHasTag(game.ID, db.TagFavorite) {
			favBtn.SetIcon(iconStarFilled)
			favBtn.tip = "Remove from favorites"
		} else {
			favBtn.SetIcon(iconStarOutline)
			favBtn.tip = "Add to favorites"
		}
		if gameHasTag(game.ID, db.TagHidden) {
			hideBtn.SetIcon(theme.VisibilityIcon())
			hideBtn.tip = "Show this game in the list again"
		} else {
			hideBtn.SetIcon(theme.VisibilityOffIcon())
			hideBtn.tip = "Hide this game"
		}
	}
	toggleTag := func(tag string) {
		gameRaw, _ := selectedGame.Get()
		game, ok := gameRaw.(db.Game)
		if !ok {
			return
		}
		var err error
		if gameHasTag(game.ID, tag) {
			err = db.RemoveTag(context.Background(), game.ID, tag)
		} else {
			err = db.AddTag(context.Background(), game.ID, tag)
		}
		if err != nil {
			showErrorDialog(win, "Could not mark the game", err)
			return
		}
		loadGameTags()
		refreshMarks(game)
		if onTagsChanged != nil {
			onTagsChanged()
		}
	}
	favBtn.OnTapped = func() { toggleTag(db.TagFavorite) }
	hideBtn.OnTapped = func() { toggleTag(db.TagHidden) }

	// The game's own artwork and the pictures from its store page are shown
	// together, the artwork first.
	gallery := newGameGallery(covers, win)

	// The description is filled once GOG has been asked about the game, and
	// stays empty for games it no longer describes.
	storeHeader := container.NewStack()

	// Until then, the pane says the lookup is running rather than sitting
	// silent: a description that never comes and one still on its way look the
	// same otherwise.
	storeStatus := widget.NewLabel("")
	storeStatus.TextStyle = fyne.TextStyle{Italic: true}
	storeStatus.Hide()

	// The overview is the game and what fetching it would take: the pictures,
	// what the store says, the options a download uses, and the ways out to the
	// web. The facts are a tab of their own, being reference rather than
	// something to read through.
	overview := container.NewVScroll(container.NewVBox(
		gallery, storeStatus, storeHeader, widget.NewSeparator(), form.options,
	))
	tabs := container.NewAppTabs(
		container.NewTabItem("Overview", overview),
		container.NewTabItem("Details", container.NewVScroll(detailsBox)),
	)

	// The buttons sit under the tabs rather than in them, so the download is
	// always in the same place and never at the far end of a scroll.
	actions := container.NewVBox(widget.NewSeparator(), form.actions)
	body := container.NewBorder(nil, actions, nil, nil, tabs)

	header := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(favBtn, hideBtn), title),
		widget.NewSeparator())

	// With no game picked out, the pane says what would fill it rather than
	// leaving half the window blank under a line of text.
	nothing := emptyState(theme.ListIcon(), "No game selected",
		"Pick one from the list to see what it is and to download it.", nil)

	return &detailsPane{
		content:      container.NewBorder(header, nil, nil, nil, container.NewStack(body, nothing)),
		empty:        nothing,
		form:         form,
		gallery:      gallery,
		title:        title,
		favorite:     favBtn,
		hide:         hideBtn,
		refreshMarks: refreshMarks,
		storeHeader:  storeHeader,
		storeStatus:  storeStatus,
		facts:        detailsBox,
		body:         body,
		tabs:         tabs,
	}
}

func createDownloadForm(win fyne.Window, authService *auth.Service, dm *DownloadManager,
	selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
) *downloadForm {
	prefs := fyne.CurrentApp().Preferences()
	downloadPathEntry := widget.NewEntry()
	downloadPathEntry.SetText(rememberedDownloadPath(prefs))
	downloadPathEntry.OnChanged = func(s string) { prefs.SetString("downloadForm.path", s) }
	downloadPathEntry.SetPlaceHolder("Enter download path")
	browseBtn := widget.NewButton("Browse...", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			downloadPathEntry.SetText(uri.Path())
		}, win)
		fd.Resize(fileDialogSize)
		fd.Show()
	})
	pathContainer := container.NewBorder(nil, nil, nil, browseBtn, downloadPathEntry)

	// Checked rather than picked from a list: a download can want English and
	// French for Windows and Linux at once. The boxes show language names but
	// store the codes the rest of gogg uses, and the first choice stands in
	// for the set wherever one value is still expected.
	onLanguagesPicked := func(names []string) {
		codes := make([]string, 0, len(names))
		for _, name := range names {
			if code, ok := languageCodes[name]; ok {
				codes = append(codes, code)
			}
		}
		prefs.SetString("downloadForm.languages", strings.Join(codes, ","))
		if len(codes) > 0 {
			prefs.SetString("downloadForm.language", codes[0])
		}
	}
	onPlatformsPicked := func(platforms []string) {
		prefs.SetString("downloadForm.platforms", strings.Join(platforms, ","))
		if len(platforms) > 0 {
			prefs.SetString("downloadForm.platform", platforms[0])
		}
	}

	langGroup := widget.NewCheckGroup(nil, nil)
	platformGroup := widget.NewCheckGroup(nil, nil)
	platformGroup.Horizontal = true

	// narrowTo restricts the choices to what a game actually offers. A zero
	// game restores the full lists.
	narrowTo := func(game db.Game) {
		var languages, platforms []string
		if parsed, err := client.ParseGameData(game.Data); err == nil {
			languages = offeredLanguages(parsed)
			platforms = offeredPlatforms(parsed)
		}
		bindCheckGroup(langGroup, languageChoices(languages),
			languageNamesFor(prefs.StringWithFallback("downloadForm.languages",
				prefs.StringWithFallback("downloadForm.language", "en"))), onLanguagesPicked)
		bindCheckGroup(platformGroup, platformGroupChoices(platforms),
			splitCSV(prefs.StringWithFallback("downloadForm.platforms",
				prefs.StringWithFallback("downloadForm.platform", "windows"))), onPlatformsPicked)
	}
	narrowTo(db.Game{})
	threadsSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, func(s string) { prefs.SetString("downloadForm.threads", s) })
	threadsSelect.SetSelected(prefs.StringWithFallback("downloadForm.threads", "5"))

	extrasCheck := widget.NewCheck("Include extras", func(b bool) { prefs.SetBool("downloadForm.extras", b) })
	extrasCheck.SetChecked(prefs.BoolWithFallback("downloadForm.extras", true))
	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) { prefs.SetBool("downloadForm.dlcs", b) })
	dlcsCheck.SetChecked(prefs.BoolWithFallback("downloadForm.dlcs", true))
	resumeCheck := widget.NewCheck("Resume downloads", func(b bool) { prefs.SetBool("downloadForm.resume", b) })
	resumeCheck.SetChecked(prefs.BoolWithFallback("downloadForm.resume", true))
	flattenCheck := widget.NewCheck("Flatten directory", func(b bool) { prefs.SetBool("downloadForm.flatten", b) })
	flattenCheck.SetChecked(prefs.BoolWithFallback("downloadForm.flatten", true))
	skipPatchesCheck := widget.NewCheck("Skip patches", func(b bool) { prefs.SetBool("downloadForm.skipPatches", b) })
	skipPatchesCheck.SetChecked(prefs.BoolWithFallback("downloadForm.skipPatches", true))
	keepLatestCheck := widget.NewCheck("Keep only latest installer", func(b bool) { prefs.SetBool("downloadForm.keepLatest", b) })
	keepLatestCheck.SetChecked(prefs.BoolWithFallback("downloadForm.keepLatest", false))
	rommCheck := widget.NewCheck("RomM folder layout (platform/game)", func(b bool) { prefs.SetBool("downloadForm.romm", b) })
	rommCheck.SetChecked(prefs.BoolWithFallback("downloadForm.romm", false))

	// Short names, because these share a line with the download button now,
	// and the pane has to fit the default window with the sidebar open.
	storeURL := ""
	storeBtn := widget.NewButton("GOG", func() {
		if parsed := parseURL(storeURL); parsed != nil {
			_ = fyne.CurrentApp().OpenURL(parsed)
		}
	})
	storeBtn.Hide()

	gogdbBtn := widget.NewButton("GOGDB", func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			return
		}
		game := gameRaw.(db.Game)
		url := fmt.Sprintf("https://www.gogdb.org/product/%d", game.ID)
		_ = fyne.CurrentApp().OpenURL(parseURL(url))
	})

	// queue turns "these games" plus whatever the form currently says into
	// queued downloads. Both the download button and Update All go through it.
	queue := func(games []db.Game) (batchResult, error) {
		if downloadPathEntry.Text == "" {
			return batchResult{}, errors.New("download path cannot be empty")
		}
		languages := append([]string{}, langGroup.Selected...)
		platforms := append([]string{}, platformGroup.Selected...)
		if len(languages) == 0 || len(platforms) == 0 {
			return batchResult{}, errors.New("pick at least one language and one platform")
		}
		// Where one value is still expected, the first choice stands in; more
		// than one platform behaves as "all" for folders and pruning.
		primaryPlatform := platforms[0]
		if len(platforms) > 1 {
			primaryPlatform = "all"
		}
		threads, _ := strconv.Atoi(threadsSelect.Selected)
		return queueDownloads(dm, games, func(game db.Game) queuedDownload {
			return queuedDownload{
				authService: authService, game: game, downloadPath: downloadPathEntry.Text,
				language: languages[0], platformName: primaryPlatform,
				languages: languages, platforms: platforms,
				extrasFlag: extrasCheck.Checked, dlcFlag: dlcsCheck.Checked,
				resumeFlag: resumeCheck.Checked, flattenFlag: flattenCheck.Checked,
				skipPatchesFlag: skipPatchesCheck.Checked, keepLatestFlag: keepLatestCheck.Checked,
				rommLayoutFlag: rommCheck.Checked, numThreads: threads,
			}
		}), nil
	}

	// targets are the games an action applies to: the ticked ones, or the
	// highlighted one when nothing is ticked.
	targets := func() []db.Game {
		if games := sel.gamesIn(catalogue()); len(games) > 0 {
			return games
		}
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			return nil
		}
		return []db.Game{gameRaw.(db.Game)}
	}

	// An icon with its meaning on hover, like the toolbar's: the row it sits
	// in has the web links, the download button, and only so much pane.
	estimateBtn := newIconButton(theme.InfoIcon(), tipEstimate, func() {
		games := targets()
		if len(games) == 0 {
			return
		}
		estimates, total := estimateSelection(games)
		showSizeEstimate(win, estimates, total)
	})

	// Named rather than left in the button, because Enter on the list starts
	// the same download the button does.
	startDownload := func() {
		games := targets()
		if len(games) == 0 {
			return
		}

		result, err := queue(games)
		if err != nil {
			showErrorDialog(win, "Cannot start download", err)
			return
		}
		if len(games) == 1 && result.Queued == 1 {
			dialog.ShowInformation("Started", fmt.Sprintf("Download for '%s' has started.", games[0].Title), win)
			return
		}
		dialog.ShowInformation("Downloads", result.summary(), win)
	}
	downloadBtn := widget.NewButtonWithIcon("Download Game", theme.DownloadIcon(), startDownload)
	downloadBtn.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Download Path", pathContainer),
		widget.NewFormItem("Platforms", platformGroup),
		widget.NewFormItem("Languages", langGroup),
		widget.NewFormItem("Threads", threadsSelect),
	)
	// Seven switches in a grid say nothing about what they do to each other.
	// Grouped, each heading answers that.
	checkboxes := container.NewVBox(
		optionGroup("What to download", extrasCheck, dlcsCheck),
		optionGroup("Which files", resumeCheck, skipPatchesCheck, keepLatestCheck),
		optionGroup("Where they go", flattenCheck, rommCheck),
	)

	relabel := func() {
		if n := sel.count(); n > 0 {
			downloadBtn.SetText(fmt.Sprintf("Download Selected (%d)", n))
			return
		}
		downloadBtn.SetText("Download Game")
	}

	return &downloadForm{
		options: container.NewVBox(form, widget.NewSeparator(), checkboxes),
		// The ways out to the web sit left, the things to do sit right, on one
		// line: what is about the game and what acts on it, at a glance. Each
		// at its own size: a button handed the whole width of the pane reads
		// as a banner rather than something to press.
		actions: container.NewHBox(
			fixedSize(storeBtn, paneCompactSize), fixedSize(gogdbBtn, paneCompactSize),
			layout.NewSpacer(),
			estimateBtn, fixedSize(downloadBtn, paneActionSize)),
		showStore: func(url string) {
			storeURL = url
			if url == "" {
				storeBtn.Hide()
				return
			}
			storeBtn.Show()
		},
		relabel:  relabel,
		queue:    queue,
		download: startDownload,
		narrowTo: narrowTo,
	}
}

// getGameDownloadDirectory says where a game's files landed, from the download
// history first and the remembered download root as a fallback.
func getGameDownloadDirectory(dm *DownloadManager, game db.Game) (string, bool) {
	if path, ok := getLastCompletedDownloadDir(dm, game.ID); ok {
		return path, true
	}
	root := fyne.CurrentApp().Preferences().String("lastUsedDownloadPath")
	if root == "" {
		return "", false
	}
	candidate := filepath.Join(root, client.SanitizePath(game.Title))
	if _, err := os.Stat(filepath.Join(candidate, "metadata.json")); err == nil {
		return candidate, true
	}
	return "", false
}

func readDownloadInfo(downloadDir string) (language, platform string) {
	infoPath := filepath.Join(downloadDir, "download_info.json")
	b, err := os.ReadFile(infoPath)
	if err != nil {
		return "", ""
	}
	var info struct {
		Language string `json:"language"`
		Platform string `json:"platform"`
	}
	if json.Unmarshal(b, &info) != nil {
		return "", ""
	}
	return info.Language, info.Platform
}

func isPatchFile(f client.PlatformFile) bool {
	name := strings.ToLower(f.Name)
	if f.ManualURL != nil {
		u := strings.ToLower(*f.ManualURL)
		if strings.Contains(u, "patch") {
			return true
		}
	}
	return strings.Contains(name, "patch")
}

func buildVersionMapExtended(g client.Game, language, platform string, includeExtras, includeDLCs, includePatches bool) map[string]string {
	m := make(map[string]string)
	add := func(prefix, pName string, files []client.PlatformFile) {
		for _, f := range files {
			if !includePatches && isPatchFile(f) {
				continue
			}
			ver := ""
			if f.Version != nil {
				ver = *f.Version
			}
			key := prefix + pName + "|" + f.Name
			m[key] = ver
		}
	}
	matchLang := func(l string) bool { return strings.EqualFold(l, language) }
	includePlatform := func(p string) bool { return platform == "all" || strings.EqualFold(p, platform) }
	for _, dl := range g.Downloads {
		if !matchLang(dl.Language) {
			continue
		}
		if includePlatform("windows") {
			add("", "windows", dl.Platforms.Windows)
		}
		if includePlatform("mac") {
			add("", "mac", dl.Platforms.Mac)
		}
		if includePlatform("linux") {
			add("", "linux", dl.Platforms.Linux)
		}
	}
	if includeExtras {
		for _, e := range g.Extras {
			m["extras|"+e.Name] = ""
		}
	}
	if includeDLCs {
		for _, dlc := range g.DLCs {
			for _, dl := range dlc.ParsedDownloads {
				if !matchLang(dl.Language) {
					continue
				}
				platforms := []struct {
					name  string
					files []client.PlatformFile
				}{{"windows", dl.Platforms.Windows}, {"mac", dl.Platforms.Mac}, {"linux", dl.Platforms.Linux}}
				for _, pf := range platforms {
					if includePlatform(pf.name) {
						add("dlc:"+client.SanitizePath(dlc.Title)+"|", pf.name, pf.files)
					}
				}
			}
			if includeExtras {
				for _, e := range dlc.Extras {
					m["dlc_extras:"+client.SanitizePath(dlc.Title)+"|"+e.Name] = ""
				}
			}
		}
	}
	return m
}

// describeAddedFile says a file was not there the last time the game was
// fetched.
func describeAddedFile(key, version string) string {
	if version == "" {
		return fileInWords(key) + ": new"
	}
	return fmt.Sprintf("%s: new, version %s", fileInWords(key), version)
}

// describeChangedFile says a file has moved on since it was fetched.
func describeChangedFile(key, from, to string) string {
	switch {
	case from == "":
		return fmt.Sprintf("%s: now version %s", fileInWords(key), to)
	case to == "":
		return fmt.Sprintf("%s: no longer has a version", fileInWords(key))
	default:
		return fmt.Sprintf("%s: %s → %s", fileInWords(key), from, to)
	}
}

// fileInWords turns the key the version map uses into the file it stands for.
// The key is a path through the platform, the DLC and the extras a file belongs
// to, which is how gogg tells two files apart rather than something to read.
func fileInWords(key string) string {
	parts := strings.Split(key, "|")
	var about []string
	switch {
	case strings.HasPrefix(parts[0], "dlc:"):
		about = append(about, strings.TrimPrefix(parts[0], "dlc:"))
		parts = parts[1:]
	case strings.HasPrefix(parts[0], "dlc_extras:"):
		about = append(about, strings.TrimPrefix(parts[0], "dlc_extras:"), "extra")
		parts = parts[1:]
	case parts[0] == "extras":
		about = append(about, "extra")
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return key
	}

	name := parts[len(parts)-1]
	// What is left in front of the name is the platform, which extras have none
	// of.
	if len(parts) > 1 {
		about = append([]string{platformInWords(parts[0])}, about...)
	}
	if len(about) == 0 {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, strings.Join(about, ", "))
}

// platformInWords names a platform the way the rest of the app does.
func platformInWords(platform string) string {
	switch platform {
	case "windows":
		return "Windows"
	case "mac":
		return "macOS"
	case "linux":
		return "Linux"
	}
	return platform
}

// showGallery puts everything there is to see of a game in the gallery: its own
// artwork, and the pictures from its store page once they are known.
func showGallery(gallery *gameGallery, game db.Game, shots []client.Screenshot) {
	pictures, selected := galleryPicturesFor(game, shots)
	gallery.show(pictures, selected)
}

// fillDetails puts a game's facts in the pane. meta is nil until GOG's store
// has been looked up, and the pane is filled twice: once without it, once with.
func fillDetails(pane *detailsPane, game db.Game, dm *DownloadManager, meta *client.GameMetadata) {
	details := gameDetails(game, dm, meta)

	pane.facts.Objects = []fyne.CanvasObject{renderGameDetails(details)}
	pane.facts.Refresh()
}

// fillStoreHeader puts the description and the link to the store page above the
// facts, where they can be read without opening anything. A game GOG does not
// describe leaves the space empty.
func fillStoreHeader(pane *detailsPane, meta *client.GameMetadata) {
	summary, markdown, storeURL := "", "", ""
	if meta != nil {
		summary, markdown, storeURL = meta.Summary, meta.SummaryMarkdown, meta.StoreURL
	}

	if header := renderStoreHeader(summary, markdown); header != nil {
		// Folded away until asked for: the pane is first of all the way to a
		// download, and a long description pushed everything below the fold.
		description := widget.NewAccordion(widget.NewAccordionItem("Description", header))
		pane.storeHeader.Objects = []fyne.CanvasObject{description}
	} else {
		pane.storeHeader.Objects = nil
	}
	pane.storeHeader.Refresh()
	pane.form.showStore(storeURL)
}

// Buttons in a box or a grid are handed the whole width they are given, which
// leaves a button of 154 points rendered at 560. These are the sizes the details
// pane hands out instead.
var (
	paneButtonSize = fyne.NewSize(160, 36)
	paneActionSize = fyne.NewSize(200, 36)
	// paneCompactSize fits a short word: the web links share the action row
	// with the download button, and every point they take is the pane's.
	paneCompactSize = fyne.NewSize(88, 36)
)

// fixedSize gives a widget a size of its own, whatever it is put inside.
func fixedSize(object fyne.CanvasObject, size fyne.Size) fyne.CanvasObject {
	return container.New(&atLeastLayout{size: size}, object)
}

// atLeastLayout lays an object out at the size it was given, or at the size it
// needs when that is larger. The size is a floor rather than a cap: a longer
// label, or a larger font chosen in Settings, must not be cut off.
type atLeastLayout struct {
	size fyne.Size
}

func (l *atLeastLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	min := l.size
	for _, object := range objects {
		min = min.Max(object.MinSize())
	}
	return min
}

func (l *atLeastLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, object := range objects {
		object.Move(fyne.NewPos(0, 0))
		object.Resize(size)
	}
}

// optionGroup is a titled set of switches, so what each one affects is clear
// from the heading rather than from its own wording alone.
func optionGroup(title string, checks ...*widget.Check) fyne.CanvasObject {
	heading := widget.NewLabel(title)
	heading.TextStyle = fyne.TextStyle{Bold: true}

	// One per line: two columns of "RomM folder layout (platform/game)" set the
	// width of the whole pane, and the pane is beside the list, not instead of
	// it.
	boxes := make([]fyne.CanvasObject, 0, len(checks)+1)
	boxes = append(boxes, heading)
	for _, check := range checks {
		boxes = append(boxes, check)
	}
	return container.NewVBox(boxes...)
}
