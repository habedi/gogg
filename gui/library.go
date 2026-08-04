package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
)

// libraryTab holds all the components of the library tab UI.
type libraryTab struct {
	content     fyne.CanvasObject
	searchEntry *widget.Entry
	// selected is the game shown in the details pane, driven by the list.
	selected binding.Untyped
	// split is the divider between the list and the details, remembered
	// between runs.
	split *container.Split
	// refresh re-syncs the catalogue, the same as the Refresh button.
	refresh func()
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
}

// computeUpdateStatus recomputes and caches status for provided games.
func computeUpdateStatus(dm *DownloadManager, games []db.Game) {
	prefs := fyne.CurrentApp().Preferences()
	includeExtrasUpdates := prefs.BoolWithFallback("downloadForm.includeExtrasUpdates", false)
	includeDLCUpdates := prefs.BoolWithFallback("downloadForm.includeDLCUpdates", false)
	scanDirs := prefs.BoolWithFallback("downloadForm.scanDirsForDownloads", true)
	includePatchUpdates := prefs.BoolWithFallback("downloadForm.includePatchUpdates", false)
	langPref := prefs.StringWithFallback("downloadForm.language", "en")
	platformPref := prefs.StringWithFallback("downloadForm.platform", "windows")
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
						diff = append(diff, "NEW: "+k+" version="+newVer)
					} else if newVer != oldVer {
						diff = append(diff, "CHANGED: "+k+" "+oldVer+" -> "+newVer)
					}
				}
				if len(diff) > 0 {
					status.HasUpdate = true
					status.Diff = diff
				}
			}
		}
		updateStatusCache[game.ID] = status
	}
	persistUpdateStatusCache()
}

// hasGameUpdateCached now reads cache
func hasGameUpdateCached(gameID int) (bool, []string) {
	st, ok := updateStatusCache[gameID]
	if !ok {
		return false, nil
	}
	return st.HasUpdate, st.Diff
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

// isGameDownloadedCached uses cache
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
	parsed, err := client.ParseGameData(game.Data)
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

// Active filters
var (
	filterDownloadedOnly bool
	filterHasUpdateOnly  bool
	filterSizeMin        int64
	filterSizeMax        int64
)

func resetFilters() {
	filterDownloadedOnly = false
	filterHasUpdateOnly = false
	filterSizeMin = 0
	filterSizeMax = 0
}

func passesFilters(game db.Game) bool {
	st, ok := updateStatusCache[game.ID]
	if filterDownloadedOnly && (!ok || !st.Downloaded) {
		return false
	}
	if filterHasUpdateOnly && (!ok || !st.HasUpdate) {
		return false
	}
	if filterSizeMin > 0 || filterSizeMax > 0 {
		sz := estimateGameSize(game)
		if filterSizeMin > 0 && sz < filterSizeMin {
			return false
		}
		if filterSizeMax > 0 && sz > filterSizeMax {
			return false
		}
	}
	return true
}

func parseSizeInput(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Accept suffix MB/GB
	lower := strings.ToLower(s)
	mult := int64(1)
	if strings.HasSuffix(lower, "mb") {
		mult = 1024 * 1024
		s = strings.TrimSpace(lower[:len(lower)-2])
	}
	if strings.HasSuffix(lower, "gb") {
		mult = 1024 * 1024 * 1024
		s = strings.TrimSpace(lower[:len(lower)-2])
	}
	if strings.HasSuffix(lower, "tb") {
		mult = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSpace(lower[:len(lower)-2])
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(v * float64(mult)))
}

// Create filter dialog button (compact UI) without tag filtering.
func newFiltersButton(refresh func()) *widget.Button {
	var dlg *dialog.CustomDialog
	btn := widget.NewButtonWithIcon("Filters", theme.SearchIcon(), func() {
		// Inputs
		downloadedChk := widget.NewCheck("Downloaded only", func(b bool) { filterDownloadedOnly = b })
		downloadedChk.SetChecked(filterDownloadedOnly)
		updateChk := widget.NewCheck("Has update", func(b bool) { filterHasUpdateOnly = b })
		updateChk.SetChecked(filterHasUpdateOnly)
		sizeMinEntry := widget.NewEntry()
		if filterSizeMin > 0 {
			sizeMinEntry.SetText(fmt.Sprintf("%.2f GB", float64(filterSizeMin)/1024/1024/1024))
		}
		sizeMaxEntry := widget.NewEntry()
		if filterSizeMax > 0 {
			sizeMaxEntry.SetText(fmt.Sprintf("%.2f GB", float64(filterSizeMax)/1024/1024/1024))
		}
		applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
			filterSizeMin = parseSizeInput(sizeMinEntry.Text)
			filterSizeMax = parseSizeInput(sizeMaxEntry.Text)
			refresh()
			dlg.Hide()
		})
		resetBtn := widget.NewButtonWithIcon("Reset", theme.ViewRefreshIcon(), func() { resetFilters(); refresh(); dlg.Hide() })
		content := container.NewVBox(
			widget.NewLabelWithStyle("Filters", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator(),
			container.NewGridWithColumns(2, widget.NewLabel("Min Size"), sizeMinEntry, widget.NewLabel("Max Size"), sizeMaxEntry),
			container.NewGridWithColumns(2, downloadedChk, updateChk),
			container.NewHBox(applyBtn, resetBtn),
		)
		dlg = dialog.NewCustom("Library Filters", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
		dlg.Resize(fyne.NewSize(400, 260))
		dlg.Show()
	})
	btn.Importance = widget.MediumImportance
	return btn
}

// LibraryTabUI modifications: remove tag editor and apply initial speed limit.
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
		content := container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.WarningIcon()),
			widget.NewLabelWithStyle("Not logged in.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Log in to GOG to see the games you own."),
			loginBtn,
		))
		return &libraryTab{
			content:     content,
			searchEntry: widget.NewEntry(),
			selected:    binding.NewUntyped(),
			refresh:     func() {},
		}
	}

	allGames, _ := db.GetCatalogue()
	gamesListBinding := binding.NewUntypedList()
	selectedGameBinding := binding.NewUntyped()
	isSortAscending := true

	gameCountLabel := widget.NewLabel("")

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Type game title to search...")
	clearSearchBtn := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		searchEntry.SetText("")
	})
	searchEntry.ActionItem = clearSearchBtn
	clearSearchBtn.Hide()

	sel := newGameSelection()
	// These are assigned once every widget they touch exists.
	var afterSelectionChange func()
	var refreshUpdatesSummary func()

	updatesLabel := widget.NewLabel("")
	updateAllBtn := widget.NewButtonWithIcon("Update All", theme.DownloadIcon(), nil)
	updateAllBtn.Importance = widget.HighImportance
	updateAllBtn.Hide()

	var gameListWidget *widget.List
	displayedGames := func() []db.Game {
		items, _ := gamesListBinding.Get()
		games := make([]db.Game, 0, len(items))
		for _, item := range items {
			games = append(games, item.(db.Game))
		}
		return games
	}

	// updateDisplayedGames decides which games are listed. Searching, sorting
	// and filtering do not change a game's status, so this does no I/O.
	updateDisplayedGames := func() {
		searchTerm := strings.ToLower(searchEntry.Text)
		displayGames := make([]db.Game, 0, len(allGames))
		for _, game := range allGames {
			if searchTerm != "" && !strings.Contains(strings.ToLower(game.Title), searchTerm) {
				continue
			}
			if !passesFilters(game) {
				continue
			}
			displayGames = append(displayGames, game)
		}

		sort.Slice(displayGames, func(i, j int) bool {
			left := strings.ToLower(displayGames[i].Title)
			right := strings.ToLower(displayGames[j].Title)
			if isSortAscending {
				return left < right
			}
			return left > right
		})

		_ = gamesListBinding.Set(untypedSlice(displayGames))
		gameCountLabel.SetText(fmt.Sprintf("%d games found", len(displayGames)))
		if searchTerm == "" {
			clearSearchBtn.Hide()
		} else {
			clearSearchBtn.Show()
		}
	}

	// recomputeStatuses refreshes the cached download and update status for the
	// whole library. It reads the filesystem and reparses every stored game, so
	// it runs only when something that can change a status happened.
	recomputeStatuses := func() {
		computeUpdateStatus(dm, allGames)
		updateDisplayedGames()
		if refreshUpdatesSummary != nil {
			refreshUpdatesSummary()
		}
	}

	searchEntry.OnChanged = func(s string) { updateDisplayedGames() }

	listContent := container.NewStack()
	gameListWidget = widget.NewListWithData(gamesListBinding,
		newGameRow,
		func(item binding.DataItem, obj fyne.CanvasObject) {
			gameRaw, _ := item.(binding.Untyped).Get()
			game, ok := gameRaw.(db.Game)
			if !ok {
				return
			}
			bindGameRow(obj, game, sel, func() { afterSelectionChange() })
		},
	)
	gameListWidget.OnSelected = func(id widget.ListItemID) {
		gameRaw, _ := gamesListBinding.GetValue(id)
		_ = selectedGameBinding.Set(gameRaw)
	}
	gameListWidget.OnUnselected = func(id widget.ListItemID) {
		_ = selectedGameBinding.Set(nil)
	}

	// Refresh icons when download tasks change
	dm.Tasks.AddListener(binding.NewDataListener(func() {
		computeUpdateStatus(dm, displayedGames()) // recalc for current displayed games
		if refreshUpdatesSummary != nil {
			refreshUpdatesSummary()
		}
		gameListWidget.Refresh()
	}))

	var refreshBtn *widget.Button
	onFinishRefresh := func() {
		allGames, _ = db.GetCatalogue()
		recomputeStatuses()
		refreshBtn.Enable()
		listContent.Objects = []fyne.CanvasObject{gameListWidget}
		listContent.Refresh()
	}

	if len(allGames) == 0 {
		placeholder := container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.InfoIcon()),
			widget.NewLabel("Your library is empty or hasn't been synced."),
			widget.NewButton("Refresh Catalogue Now", func() {
				refreshBtn.Disable()
				RefreshCatalogueAction(win, authService, onFinishRefresh)
			}),
		))
		listContent.Add(placeholder)
	} else {
		listContent.Add(gameListWidget)
	}
	// Load any status cached by an earlier session before recomputing, so stale
	// entries cannot overwrite fresh ones.
	initUpdateStatusPersistence()
	recomputeStatuses()

	refreshBtn = widget.NewButtonWithIcon("Refresh", theme.ViewRefreshIcon(), func() {
		searchEntry.SetText("")
		refreshBtn.Disable()
		RefreshCatalogueAction(win, authService, onFinishRefresh)
	})

	var exportBtn *widget.Button
	exportBtn = widget.NewButtonWithIcon("Export", theme.DocumentSaveIcon(), func() {
		popup := widget.NewPopUpMenu(fyne.NewMenu("",
			fyne.NewMenuItem("Export Game List as CSV", func() { ExportCatalogueAction(win, "csv") }),
			fyne.NewMenuItem("Export Full Catalogue as JSON", func() { ExportCatalogueAction(win, "json") }),
		), win.Canvas())
		popup.ShowAtPosition(win.Content().Position().Add(fyne.NewPos(exportBtn.Position().X, exportBtn.Position().Y+exportBtn.Size().Height)))
	})

	var sortBtn *widget.Button
	sortBtn = widget.NewButton("Sort A-Z", func() {
		isSortAscending = !isSortAscending
		if isSortAscending {
			sortBtn.SetText("Sort A-Z")
		} else {
			sortBtn.SetText("Sort Z-A")
		}
		updateDisplayedGames()
		gameListWidget.Refresh()
	})

	// Preferences toggles for update detection scope (replaced by compact settings button)
	prefs := fyne.CurrentApp().Preferences()
	settingsBtn := newUpdateSettingsButton(prefs, dm, recomputeStatuses)
	filtersBtn := newFiltersButton(updateDisplayedGames)
	// Compact toolbar now
	// The buttons scroll rather than forcing a minimum width on the window; the
	// counts stay pinned to the right.
	toolbarButtons := container.NewHScroll(
		container.NewHBox(refreshBtn, exportBtn, sortBtn, settingsBtn, filtersBtn, updateAllBtn))
	toolbar := container.NewBorder(nil, nil, nil,
		container.NewHBox(updatesLabel, gameCountLabel), toolbarButtons)
	selectionLabel := widget.NewLabel("")
	// Changing many rows at once needs the list redrawn; a row the user ticked
	// themselves already shows the right state.
	applyBulkSelection := func(change func()) {
		change()
		afterSelectionChange()
		gameListWidget.Refresh()
	}
	selectAllBtn := widget.NewButton("Select All Shown", func() {
		applyBulkSelection(func() { sel.selectAll(displayedGames()) })
	})
	clearSelectionBtn := widget.NewButton("Clear Selection", func() {
		applyBulkSelection(sel.clear)
	})
	selectionControls := container.NewHBox(selectAllBtn, clearSelectionBtn, layout.NewSpacer(), selectionLabel)

	leftTopContainer := container.NewVBox(searchEntry, selectionControls, widget.NewSeparator())
	leftPane := container.NewBorder(leftTopContainer, toolbar, nil, nil, listContent)

	detailTitle := NewCopyableLabel("Select a game from the list")
	detailTitle.Alignment = fyne.TextAlignCenter
	detailTitle.TextStyle = fyne.TextStyle{Bold: true}

	detailsBox := container.NewVBox()
	accordion, form := createDetailsAccordion(win, authService, dm, selectedGameBinding,
		sel, func() []db.Game { return allGames }, detailsBox)

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
	topBox := container.NewVBox(detailTitle, widget.NewSeparator())
	// The details and options can be taller than the window, and a window
	// cannot be smaller than its content, so the pane scrolls instead.
	rightPane := container.NewBorder(topBox, nil, nil, nil, container.NewVScroll(accordion))
	accordion.Hide()

	selectedGameBinding.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGameBinding.Get()
		if gameRaw == nil {
			accordion.Hide()
			form.narrowTo(db.Game{})
			detailTitle.SetText("Select a game from the list")
			topBox.Objects = []fyne.CanvasObject{detailTitle, widget.NewSeparator()}
			topBox.Refresh()
			return
		}
		game := gameRaw.(db.Game)
		detailTitle.SetText(game.Title)
		detailsBox.Objects = []fyne.CanvasObject{renderGameDetails(gameDetails(game, dm))}
		detailsBox.Refresh()
		form.narrowTo(game)
		accordion.Show()

		topBox.Objects = []fyne.CanvasObject{detailTitle, widget.NewSeparator()}
		topBox.Refresh()
	}))
	// AddListener invokes the listener once on registration, and the status
	// worked out above is still valid at that point.
	catalogueJustRegistered := true
	catalogueUpdated.AddListener(binding.NewDataListener(func() {
		if catalogueJustRegistered {
			catalogueJustRegistered = false
			return
		}
		clearPersistedUpdateStatus()
		sizeCache = make(map[sizeCacheKey]int64)
		recomputeStatuses()
	}))
	split := container.NewHSplit(leftPane, rightPane)
	split.Offset = loadWindowState(prefs).SplitOffset

	return &libraryTab{
		content:     split,
		searchEntry: searchEntry,
		selected:    selectedGameBinding,
		split:       split,
		refresh:     func() { refreshBtn.OnTapped() },
	}
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

// downloadForm exposes what the rest of the library needs from the options pane.
type downloadForm struct {
	content fyne.CanvasObject
	// relabel brings the download button in line with the current selection.
	relabel func()
	// queue starts downloads for the given games with the options on screen.
	queue func(games []db.Game) (batchResult, error)
	// narrowTo restricts the language and platform choices to what a game
	// offers; a zero game restores the full lists.
	narrowTo func(game db.Game)
}

// createDetailsAccordion builds the details pane. detailsBox is filled with the
// selected game's facts by the caller.
func createDetailsAccordion(win fyne.Window, authService *auth.Service, dm *DownloadManager,
	selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
	detailsBox *fyne.Container,
) (*widget.Accordion, *downloadForm) {
	form := createDownloadForm(win, authService, dm, selectedGame, sel, catalogue)
	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Game Details", detailsBox),
		widget.NewAccordionItem("Download Options", form.content),
	)
	accordion.MultiOpen = true
	accordion.Open(0)
	accordion.Open(1)
	return accordion, form
}

func createDownloadForm(win fyne.Window, authService *auth.Service, dm *DownloadManager,
	selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
) *downloadForm {
	prefs := fyne.CurrentApp().Preferences()
	downloadPathEntry := widget.NewEntry()
	lastUsedPath := prefs.String("lastUsedDownloadPath")
	if lastUsedPath == "" {
		lastUsedPath = prefs.StringWithFallback("downloadForm.path", "")
	}
	downloadPathEntry.SetText(lastUsedPath)
	downloadPathEntry.OnChanged = func(s string) { prefs.SetString("downloadForm.path", s) }
	downloadPathEntry.SetPlaceHolder("Enter download path")
	browseBtn := widget.NewButton("Browse...", func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			downloadPathEntry.SetText(uri.Path())
		}, win)
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})
	pathContainer := container.NewBorder(nil, nil, nil, browseBtn, downloadPathEntry)

	// The selects show language names but store the codes the rest of gogg uses.
	onLanguagePicked := func(name string) {
		if code, ok := languageCodes[name]; ok {
			prefs.SetString("downloadForm.language", code)
		}
	}
	onPlatformPicked := func(platform string) { prefs.SetString("downloadForm.platform", platform) }

	langSelect := widget.NewSelect(nil, onLanguagePicked)
	platformSelect := widget.NewSelect(nil, onPlatformPicked)

	// narrowTo restricts the choices to what a game actually offers. A zero
	// game restores the full lists.
	narrowTo := func(game db.Game) {
		var languages, platforms []string
		if parsed, err := client.ParseGameData(game.Data); err == nil {
			languages = offeredLanguages(parsed)
			platforms = offeredPlatforms(parsed)
		}
		bindSelect(langSelect, languageChoices(languages),
			client.GameLanguages[prefs.StringWithFallback("downloadForm.language", "en")], onLanguagePicked)
		bindSelect(platformSelect, platformChoices(platforms),
			prefs.StringWithFallback("downloadForm.platform", "windows"), onPlatformPicked)
	}
	narrowTo(db.Game{})
	threadsSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, func(s string) { prefs.SetString("downloadForm.threads", s) })
	threadsSelect.SetSelected(prefs.StringWithFallback("downloadForm.threads", "5"))

	extrasCheck := widget.NewCheck("Include Extras", func(b bool) { prefs.SetBool("downloadForm.extras", b) })
	extrasCheck.SetChecked(prefs.BoolWithFallback("downloadForm.extras", true))
	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) { prefs.SetBool("downloadForm.dlcs", b) })
	dlcsCheck.SetChecked(prefs.BoolWithFallback("downloadForm.dlcs", true))
	resumeCheck := widget.NewCheck("Resume Downloads", func(b bool) { prefs.SetBool("downloadForm.resume", b) })
	resumeCheck.SetChecked(prefs.BoolWithFallback("downloadForm.resume", true))
	flattenCheck := widget.NewCheck("Flatten Directory", func(b bool) { prefs.SetBool("downloadForm.flatten", b) })
	flattenCheck.SetChecked(prefs.BoolWithFallback("downloadForm.flatten", true))
	skipPatchesCheck := widget.NewCheck("Skip Patches", func(b bool) { prefs.SetBool("downloadForm.skipPatches", b) })
	skipPatchesCheck.SetChecked(prefs.BoolWithFallback("downloadForm.skipPatches", true))
	keepLatestCheck := widget.NewCheck("Keep only latest installer", func(b bool) { prefs.SetBool("downloadForm.keepLatest", b) })
	keepLatestCheck.SetChecked(prefs.BoolWithFallback("downloadForm.keepLatest", false))
	rommCheck := widget.NewCheck("RomM folder layout (platform/game)", func(b bool) { prefs.SetBool("downloadForm.romm", b) })
	rommCheck.SetChecked(prefs.BoolWithFallback("downloadForm.romm", false))

	gogdbBtn := widget.NewButtonWithIcon("View on gogdb.org", theme.SearchIcon(), func() {
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
		threads, _ := strconv.Atoi(threadsSelect.Selected)
		return queueDownloads(dm, games, func(game db.Game) queuedDownload {
			return queuedDownload{
				authService: authService, game: game, downloadPath: downloadPathEntry.Text,
				language: langSelect.Selected, platformName: platformSelect.Selected,
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

	estimateBtn := widget.NewButtonWithIcon("Estimate Size", theme.InfoIcon(), func() {
		games := targets()
		if len(games) == 0 {
			return
		}
		estimates, total := estimateSelection(games)
		showSizeEstimate(win, estimates, total)
	})

	downloadBtn := widget.NewButtonWithIcon("Download Game", theme.DownloadIcon(), func() {
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
	})
	downloadBtn.Importance = widget.HighImportance

	form := widget.NewForm(
		widget.NewFormItem("Download Path", pathContainer),
		widget.NewFormItem("Platform", platformSelect),
		widget.NewFormItem("Language", langSelect),
		widget.NewFormItem("Threads", threadsSelect),
	)
	checkboxes := container.New(layout.NewGridLayout(2), extrasCheck, dlcsCheck, resumeCheck, flattenCheck, skipPatchesCheck, keepLatestCheck, rommCheck)

	relabel := func() {
		if n := sel.count(); n > 0 {
			downloadBtn.SetText(fmt.Sprintf("Download Selected (%d)", n))
			return
		}
		downloadBtn.SetText("Download Game")
	}

	return &downloadForm{
		content: container.NewVBox(form, checkboxes, layout.NewSpacer(),
			container.NewGridWithColumns(2, gogdbBtn, estimateBtn), downloadBtn),
		relabel:  relabel,
		queue:    queue,
		narrowTo: narrowTo,
	}
}

func newUpdateSettingsButton(prefs fyne.Preferences, dm *DownloadManager, refresh func()) *widget.Button {
	btn := widget.NewButtonWithIcon("Update Settings", theme.SettingsIcon(), func() {
		extrasUpd := widget.NewCheck("Include Extras in update check", func(b bool) { prefs.SetBool("downloadForm.includeExtrasUpdates", b); refresh() })
		extrasUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includeExtrasUpdates", false))
		dlcUpd := widget.NewCheck("Include DLCs in update check", func(b bool) { prefs.SetBool("downloadForm.includeDLCUpdates", b); refresh() })
		dlcUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includeDLCUpdates", false))
		patchUpd := widget.NewCheck("Include patches", func(b bool) { prefs.SetBool("downloadForm.includePatchUpdates", b); refresh() })
		patchUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includePatchUpdates", false))
		scanDirs := widget.NewCheck("Scan folders when history missing", func(b bool) { prefs.SetBool("downloadForm.scanDirsForDownloads", b); refresh() })
		scanDirs.SetChecked(prefs.BoolWithFallback("downloadForm.scanDirsForDownloads", true))

		content := container.NewVBox(
			widget.NewLabelWithStyle("Update Detection Options", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), widget.NewSeparator(), extrasUpd, dlcUpd, patchUpd, scanDirs,
		)
		d := dialog.NewCustom("Update Settings", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
		d.Resize(fyne.NewSize(380, 320))
		d.Show()
	})
	btn.Importance = widget.MediumImportance
	return btn
}

// FIX tag buttons capture
// Adjust tag editing to use proper closure - already handled by tt variable

// Helper functions re-added after refactor removal
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
