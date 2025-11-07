package gui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		if task.ID == gameID && task.State == StateCompleted {
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
		if task.ID != gameID || task.State != StateCompleted {
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
				lang := langPref
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

func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager) *libraryTab {
	token, _ := db.GetTokenRecord()
	if token == nil {
		content := container.NewCenter(container.NewVBox(
			widget.NewIcon(theme.WarningIcon()),
			widget.NewLabelWithStyle("Not logged in.", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			widget.NewLabel("Please run 'gogg login' from your terminal to authenticate."),
		))
		return &libraryTab{content: content, searchEntry: widget.NewEntry()} // Return dummy entry
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

	var gameListWidget *widget.List
	updateDisplayedGames := func() {
		searchTerm := strings.ToLower(searchEntry.Text)
		displayGames := make([]db.Game, len(allGames))
		copy(displayGames, allGames)

		if isSortAscending {
			sort.Slice(displayGames, func(i, j int) bool {
				return strings.ToLower(displayGames[i].Title) < strings.ToLower(displayGames[j].Title)
			})
		} else {
			sort.Slice(displayGames, func(i, j int) bool {
				return strings.ToLower(displayGames[i].Title) > strings.ToLower(displayGames[j].Title)
			})
		}

		if searchTerm != "" {
			filtered := make([]db.Game, 0)
			for _, game := range displayGames {
				if strings.Contains(strings.ToLower(game.Title), searchTerm) {
					filtered = append(filtered, game)
				}
			}
			displayGames = filtered
		}

		_ = gamesListBinding.Set(untypedSlice(displayGames))
		// Recompute cache only for displayed games for efficiency
		computeUpdateStatus(dm, displayGames)
		gameCountLabel.SetText(fmt.Sprintf("%d games found", len(displayGames)))
		if searchTerm == "" {
			clearSearchBtn.Hide()
		} else {
			clearSearchBtn.Show()
		}
	}

	searchEntry.OnChanged = func(s string) { updateDisplayedGames() }

	listContent := container.NewStack()
	gameListWidget = widget.NewListWithData(gamesListBinding,
		func() fyne.CanvasObject {
			// downloaded tick + update button
			iconDownloaded := widget.NewIcon(theme.ConfirmIcon())
			iconDownloaded.Hide()
			updateBtn := widget.NewButtonWithIcon("", theme.DownloadIcon(), nil)
			updateBtn.Hide()
			updateBtn.Importance = widget.LowImportance
			updateBtn.SetText("")
			label := widget.NewLabel("Game Title")
			icons := container.NewHBox(iconDownloaded, updateBtn)
			return container.NewHBox(icons, label)
		},
		func(item binding.DataItem, obj fyne.CanvasObject) {
			gameRaw, _ := item.(binding.Untyped).Get()
			game := gameRaw.(db.Game)

			hbox := obj.(*fyne.Container)
			iconsBox := hbox.Objects[0].(*fyne.Container)
			iconDownloaded := iconsBox.Objects[0].(*widget.Icon)
			updateBtn := iconsBox.Objects[1].(*widget.Button)
			label := hbox.Objects[1].(*widget.Label)

			label.SetText(game.Title)

			if isGameDownloadedCached(game.ID) {
				iconDownloaded.Show()
				hasUpd, diff := hasGameUpdateCached(game.ID)
				if hasUpd {
					updateBtn.Show()
					updateBtn.SetText(fmt.Sprintf("%d", len(diff)))
					updateBtn.OnTapped = func() {
						content := container.NewVBox()
						for _, line := range diff {
							content.Add(widget.NewLabel(line))
						}
						dialog.ShowCustom("Update details", "Close", container.NewVScroll(content), fyne.CurrentApp().Driver().AllWindows()[0])
					}
				} else {
					updateBtn.Hide()
					updateBtn.SetText("")
				}
			} else {
				iconDownloaded.Hide()
				updateBtn.Hide()
			}
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
		computeUpdateStatus(dm, func() []db.Game { // recalc for current displayed games
			items, _ := gamesListBinding.Get()
			gs := make([]db.Game, 0, len(items))
			for _, it := range items {
				gs = append(gs, it.(db.Game))
			}
			return gs
		}())
		gameListWidget.Refresh()
	}))

	var refreshBtn *widget.Button
	onFinishRefresh := func() {
		allGames, _ = db.GetCatalogue()
		updateDisplayedGames()
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
	updateDisplayedGames()

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
	settingsBtn := newUpdateSettingsButton(prefs, updateDisplayedGames)
	// Compact toolbar now
	toolbar := container.NewHBox(refreshBtn, exportBtn, sortBtn, settingsBtn, layout.NewSpacer(), gameCountLabel)
	leftTopContainer := container.NewVBox(searchEntry, widget.NewSeparator())
	leftPane := container.NewBorder(leftTopContainer, toolbar, nil, nil, listContent)

	detailTitle := NewCopyableLabel("Select a game from the list")
	detailTitle.Alignment = fyne.TextAlignCenter
	detailTitle.TextStyle = fyne.TextStyle{Bold: true}

	accordion := createDetailsAccordion(win, authService, dm, selectedGameBinding)
	rightPane := container.NewBorder(
		container.NewVBox(detailTitle, widget.NewSeparator()),
		nil, nil, nil,
		accordion,
	)
	accordion.Hide()

	selectedGameBinding.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGameBinding.Get()
		if gameRaw == nil {
			accordion.Hide()
			detailTitle.SetText("Select a game from the list")
			return
		}
		game := gameRaw.(db.Game)
		detailTitle.SetText(game.Title)
		accordion.Show()
	}))

	// Removed inline checkboxes; initialization of persistence unchanged
	initUpdateStatusPersistence()
	catalogueUpdated.AddListener(binding.NewDataListener(func() {
		clearPersistedUpdateStatus()
	}))

	return &libraryTab{
		content:     container.NewHSplit(leftPane, rightPane),
		searchEntry: searchEntry,
	}
}

func untypedSlice(games []db.Game) []interface{} {
	out := make([]interface{}, len(games))
	for i, g := range games {
		out[i] = g
	}
	return out
}

func createDetailsAccordion(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) *widget.Accordion {
	detailsLabel := widget.NewLabel("Game details will appear here.")
	detailsLabel.Wrapping = fyne.TextWrapWord

	downloadForm := createDownloadForm(win, authService, dm, selectedGame)

	accordion := widget.NewAccordion(
		widget.NewAccordionItem("Download Options", downloadForm),
	)
	accordion.Open(0)

	selectedGame.AddListener(binding.NewDataListener(func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			detailsLabel.SetText("Select a game to see its details.")
			return
		}
		game := gameRaw.(db.Game)

		var gameDetails map[string]interface{}
		if err := json.Unmarshal([]byte(game.Data), &gameDetails); err != nil {
			detailsLabel.SetText("Error parsing game details.")
			return
		}

		desc, ok := gameDetails["description"].(map[string]interface{})
		if ok {
			fullDesc, _ := desc["full"].(string)
			detailsLabel.SetText(fullDesc)
		} else {
			detailsLabel.SetText("No description available.")
		}
	}))

	return accordion
}

func createDownloadForm(win fyne.Window, authService *auth.Service, dm *DownloadManager, selectedGame binding.Untyped) fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()

	downloadPathEntry := widget.NewEntry()
	lastUsedPath := prefs.String("lastUsedDownloadPath")
	if lastUsedPath == "" {
		lastUsedPath = prefs.StringWithFallback("downloadForm.path", "")
	}
	downloadPathEntry.SetText(lastUsedPath)

	downloadPathEntry.OnChanged = func(s string) {
		prefs.SetString("downloadForm.path", s)
	}
	downloadPathEntry.SetPlaceHolder("Enter download path")

	browseBtn := widget.NewButton("Browse...", func() {
		folderDialog := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			downloadPathEntry.SetText(uri.Path())
		}, win)
		folderDialog.Resize(fyne.NewSize(800, 600))
		folderDialog.Show()
	})
	pathContainer := container.NewBorder(nil, nil, nil, browseBtn, downloadPathEntry)

	langCodes := make([]string, 0, len(client.GameLanguages))
	for code := range client.GameLanguages {
		langCodes = append(langCodes, code)
	}
	sort.Strings(langCodes)
	langSelect := widget.NewSelect(langCodes, func(s string) {
		prefs.SetString("downloadForm.language", s)
	})
	langSelect.SetSelected(prefs.StringWithFallback("downloadForm.language", "en"))

	platformSelect := widget.NewSelect([]string{"windows", "mac", "linux", "all"}, func(s string) {
		prefs.SetString("downloadForm.platform", s)
	})
	platformSelect.SetSelected(prefs.StringWithFallback("downloadForm.platform", "windows"))

	threadOptions := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}
	threadsSelect := widget.NewSelect(threadOptions, func(s string) {
		prefs.SetString("downloadForm.threads", s)
	})
	threadsSelect.SetSelected(prefs.StringWithFallback("downloadForm.threads", "5"))

	extrasCheck := widget.NewCheck("Include Extras", func(b bool) {
		prefs.SetBool("downloadForm.extras", b)
	})
	extrasCheck.SetChecked(prefs.BoolWithFallback("downloadForm.extras", true))

	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) {
		prefs.SetBool("downloadForm.dlcs", b)
	})
	dlcsCheck.SetChecked(prefs.BoolWithFallback("downloadForm.dlcs", true))

	resumeCheck := widget.NewCheck("Resume Downloads", func(b bool) {
		prefs.SetBool("downloadForm.resume", b)
	})
	resumeCheck.SetChecked(prefs.BoolWithFallback("downloadForm.resume", true))

	flattenCheck := widget.NewCheck("Flatten Directory", func(b bool) {
		prefs.SetBool("downloadForm.flatten", b)
	})
	flattenCheck.SetChecked(prefs.BoolWithFallback("downloadForm.flatten", true))

	skipPatchesCheck := widget.NewCheck("Skip Patches", func(b bool) {
		prefs.SetBool("downloadForm.skipPatches", b)
	})
	skipPatchesCheck.SetChecked(prefs.BoolWithFallback("downloadForm.skipPatches", true))

	gogdbBtn := widget.NewButtonWithIcon("View on gogdb.org", theme.SearchIcon(), nil)
	gogdbBtn.OnTapped = func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			return
		}
		game := gameRaw.(db.Game)
		gogdbURL := fmt.Sprintf("https://www.gogdb.org/product/%d", game.ID)
		if err := fyne.CurrentApp().OpenURL(parseURL(gogdbURL)); err != nil {
			dialog.ShowError(fmt.Errorf("failed to open gogdb URL: %w", err), win)
		}
	}

	downloadBtn := widget.NewButtonWithIcon("Download Game", theme.DownloadIcon(), nil)
	downloadBtn.Importance = widget.HighImportance
	downloadBtn.OnTapped = func() {
		if downloadPathEntry.Text == "" {
			showErrorDialog(win, "Download path cannot be empty.", nil)
			return
		}

		gameRaw, _ := selectedGame.Get()
		game := gameRaw.(db.Game)
		threads, _ := strconv.Atoi(threadsSelect.Selected)
		langFull := client.GameLanguages[langSelect.Selected]

		err := executeDownload(
			authService, dm, game,
			downloadPathEntry.Text, langFull, platformSelect.Selected,
			extrasCheck.Checked, dlcsCheck.Checked, resumeCheck.Checked,
			flattenCheck.Checked, skipPatchesCheck.Checked,
			threads,
		)

		if err != nil {
			if errors.Is(err, ErrDownloadInProgress) {
				dialog.ShowInformation("In Progress", "This game is already being downloaded.", win)
			} else {
				showErrorDialog(win, "Failed to start download", err)
			}
		} else {
			dialog.ShowInformation("Started", fmt.Sprintf("Download for '%s' has started.", game.Title), win)
		}
	}

	form := widget.NewForm(
		widget.NewFormItem("Download Path", pathContainer),
		widget.NewFormItem("Platform", platformSelect),
		widget.NewFormItem("Language", langSelect),
		widget.NewFormItem("Threads", threadsSelect),
	)

	checkboxes := container.New(layout.NewGridLayout(3), extrasCheck, dlcsCheck, skipPatchesCheck, resumeCheck, flattenCheck)

	return container.NewVBox(form, checkboxes, layout.NewSpacer(), gogdbBtn, downloadBtn)
}

func readDownloadInfo(downloadDir string) (language, platform string) {
	infoPath := downloadDir + string(os.PathSeparator) + "download_info.json"
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

// isPatchFile helps exclude patch files when comparing installer versions.
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

// buildVersionMapExtended optionally includes extras, DLC installers, and optionally patches.
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
				}{
					{"windows", dl.Platforms.Windows}, {"mac", dl.Platforms.Mac}, {"linux", dl.Platforms.Linux},
				}
				for _, pf := range platforms {
					if !includePlatform(pf.name) {
						continue
					}
					add("dlc:"+client.SanitizePath(dlc.Title)+"|", pf.name, pf.files)
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

// getGameDownloadDirectory tries history first then scans root path for metadata.json.
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

// newUpdateSettingsButton creates a compact button that opens a dialog with update scope toggles.
func newUpdateSettingsButton(prefs fyne.Preferences, refresh func()) *widget.Button {
	btn := widget.NewButtonWithIcon("Update Settings", theme.SettingsIcon(), func() {
		extrasUpd := widget.NewCheck("Include Extras in update check", func(b bool) {
			prefs.SetBool("downloadForm.includeExtrasUpdates", b)
			refresh()
		})
		extrasUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includeExtrasUpdates", false))

		dlcUpd := widget.NewCheck("Include DLCs in update check", func(b bool) {
			prefs.SetBool("downloadForm.includeDLCUpdates", b)
			refresh()
		})
		dlcUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includeDLCUpdates", false))

		patchUpd := widget.NewCheck("Include patches", func(b bool) {
			prefs.SetBool("downloadForm.includePatchUpdates", b)
			refresh()
		})
		patchUpd.SetChecked(prefs.BoolWithFallback("downloadForm.includePatchUpdates", false))

		scanDirs := widget.NewCheck("Scan folders when history missing", func(b bool) {
			prefs.SetBool("downloadForm.scanDirsForDownloads", b)
			refresh()
		})
		scanDirs.SetChecked(prefs.BoolWithFallback("downloadForm.scanDirsForDownloads", true))

		content := container.NewVBox(
			widget.NewLabelWithStyle("Update Detection Options", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			widget.NewSeparator(),
			extrasUpd,
			dlcUpd,
			patchUpd,
			scanDirs,
		)
		d := dialog.NewCustom("Update Settings", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
		d.Resize(fyne.NewSize(380, 260))
		d.Show()
	})
	btn.Importance = widget.MediumImportance
	return btn
}
