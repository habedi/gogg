package gui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/auth"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
)

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
	st stores, state *libraryState, selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
	detailsBox *fyne.Container, covers *coverCache, onTagsChanged func(),
) *detailsPane {
	form := createDownloadForm(win, authService, dm, state, selectedGame, sel, catalogue)

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
		if state.hasTag(game.ID, db.TagFavorite) {
			favBtn.SetIcon(iconStarFilled)
			favBtn.tip = "Remove from favorites"
		} else {
			favBtn.SetIcon(iconStarOutline)
			favBtn.tip = "Add to favorites"
		}
		if state.hasTag(game.ID, db.TagHidden) {
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
		if state.hasTag(game.ID, tag) {
			err = st.tags.Remove(context.Background(), game.ID, tag)
		} else {
			err = st.tags.Add(context.Background(), game.ID, tag)
		}
		if err != nil {
			showErrorDialog(win, "Could not mark the game", err)
			return
		}
		state.loadTags(st.tags)
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
	state *libraryState, selectedGame binding.Untyped, sel *gameSelection, catalogue func() []db.Game,
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
			prefs.SetString(prefFormLanguage, codes[0])
		}
	}
	// The boxes show platform names but store the keys the rest of gogg
	// downloads with, the same way the language boxes store codes.
	onPlatformsPicked := func(labels []string) {
		keys := platformKeys(labels)
		prefs.SetString("downloadForm.platforms", strings.Join(keys, ","))
		if len(keys) > 0 {
			prefs.SetString(prefFormPlatform, keys[0])
		}
	}

	langGrid := newCheckGrid(3)
	platformGroup := widget.NewCheckGroup(nil, nil)
	platformGroup.Horizontal = true

	// The download button is assigned below, but narrowTo, which runs on
	// every selection, has to reach it to enable or disable it.
	var downloadBtn *widget.Button
	shownDownloadable := true

	// refreshDownloadButton keeps the button's label and enabled state in
	// line with the selection: a batch is always startable, and a single
	// game only when GOG has files for it to download.
	refreshDownloadButton := func() {
		if downloadBtn == nil {
			return
		}
		if n := sel.count(); n > 0 {
			downloadBtn.SetText(fmt.Sprintf("Download (%d)", n))
			downloadBtn.Enable()
			return
		}
		downloadBtn.SetText("Download")
		if shownDownloadable {
			downloadBtn.Enable()
		} else {
			downloadBtn.Disable()
		}
	}

	// narrowTo restricts the choices to what a game actually offers. A zero
	// game restores the full lists.
	narrowTo := func(game db.Game) {
		var languages, platforms []string
		if parsed, err := client.ParseGameData(game.Data); err == nil {
			languages = offeredLanguages(parsed)
			platforms = offeredPlatforms(parsed)
		}
		// No platforms means GOG serves no installer files, so there is
		// nothing to download; a zero game (no selection) is not disabled,
		// its button is hidden with the rest of the pane.
		shownDownloadable = game.ID == 0 || len(platforms) > 0
		bindCheckGrid(langGrid, languageChoices(languages),
			languageNamesFor(prefs.StringWithFallback("downloadForm.languages",
				formLanguage(prefs))), onLanguagesPicked)
		bindCheckGroup(platformGroup, platformGroupLabels(platforms),
			platformLabels(splitCSV(prefs.StringWithFallback("downloadForm.platforms",
				formPlatform(prefs)))), onPlatformsPicked)
		refreshDownloadButton()
	}
	narrowTo(db.Game{})
	threadsSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, func(s string) { prefs.SetString("downloadForm.threads", s) })
	threadsSelect.SetSelected(prefs.StringWithFallback("downloadForm.threads", "5"))
	connectionsSelect := widget.NewSelect([]string{"1", "2", "4", "8"}, func(s string) { prefs.SetString("downloadForm.connections", s) })
	connectionsSelect.SetSelected(prefs.StringWithFallback("downloadForm.connections", "1"))

	extrasCheck := widget.NewCheck("Include extras", func(b bool) { prefs.SetBool(prefFormExtras, b) })
	extrasCheck.SetChecked(formExtras(prefs))
	dlcsCheck := widget.NewCheck("Include DLCs", func(b bool) { prefs.SetBool(prefFormDLCs, b) })
	dlcsCheck.SetChecked(formDLCs(prefs))
	resumeCheck := widget.NewCheck("Resume downloads", func(b bool) { prefs.SetBool("downloadForm.resume", b) })
	resumeCheck.SetChecked(prefs.BoolWithFallback("downloadForm.resume", true))
	flattenCheck := widget.NewCheck("Flatten directory", func(b bool) { prefs.SetBool("downloadForm.flatten", b) })
	flattenCheck.SetChecked(prefs.BoolWithFallback("downloadForm.flatten", true))
	skipPatchesCheck := widget.NewCheck("Skip patches", func(b bool) { prefs.SetBool("downloadForm.skipPatches", b) })
	skipPatchesCheck.SetChecked(prefs.BoolWithFallback("downloadForm.skipPatches", true))
	keepLatestCheck := widget.NewCheck("Keep only latest installer", func(b bool) { prefs.SetBool("downloadForm.keepLatest", b) })
	keepLatestCheck.SetChecked(prefs.BoolWithFallback("downloadForm.keepLatest", false))
	// The two special layouts contradict each other, so ticking one clears
	// the other.
	var rommCheck, lutrisCheck *widget.Check
	rommCheck = widget.NewCheck("RomM folder layout (platform/game)", func(b bool) {
		prefs.SetBool("downloadForm.romm", b)
		if b {
			lutrisCheck.SetChecked(false)
		}
	})
	lutrisCheck = widget.NewCheck("Lutris cache layout (game/gog)", func(b bool) {
		prefs.SetBool("downloadForm.lutris", b)
		if b {
			rommCheck.SetChecked(false)
		}
	})
	rommCheck.SetChecked(prefs.BoolWithFallback("downloadForm.romm", false))
	lutrisCheck.SetChecked(prefs.BoolWithFallback("downloadForm.lutris", false))

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
		languages := langGrid.selected()
		// The boxes carry display names; downloads want the keys.
		platforms := platformKeys(platformGroup.Selected)
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
		connections, _ := strconv.Atoi(connectionsSelect.Selected)
		return queueDownloads(dm, games, func(game db.Game) queuedDownload {
			return queuedDownload{
				authService: authService, game: game, downloadPath: downloadPathEntry.Text,
				language: languages[0], platformName: primaryPlatform,
				languages: languages, platforms: platforms,
				extrasFlag: extrasCheck.Checked, dlcFlag: dlcsCheck.Checked,
				resumeFlag: resumeCheck.Checked, flattenFlag: flattenCheck.Checked,
				skipPatchesFlag: skipPatchesCheck.Checked, keepLatestFlag: keepLatestCheck.Checked,
				rommLayoutFlag: rommCheck.Checked, lutrisLayoutFlag: lutrisCheck.Checked,
				numThreads:  threads,
				connections: connections,
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

	// Labeled rather than icon-only: a new user should not have to hover to
	// learn what "estimate the size" and "back up saves" are. They sit on
	// their own row above the download button, so the labels have the room.
	estimateBtn := widget.NewButtonWithIcon("Size", theme.InfoIcon(), func() {
		games := targets()
		if len(games) == 0 {
			return
		}
		estimates, total := estimateSelection(state, games)
		showSizeEstimate(win, estimates, total)
	})

	savesBtn := widget.NewButtonWithIcon("Saves", theme.DocumentSaveIcon(), func() {
		gameRaw, _ := selectedGame.Get()
		if gameRaw == nil {
			return
		}
		if downloadPathEntry.Text == "" {
			showErrorDialog(win, "Could not back up saves", errors.New("download path cannot be empty"))
			return
		}
		game := gameRaw.(db.Game)
		outputDir := filepath.Join(downloadPathEntry.Text, "saves", client.SanitizePath(game.Title))
		backupSaves(win, authService, game, outputDir)
	})

	// Named rather than left in the button, because Enter on the list starts
	// the same download the button does.
	startDownload := func() {
		games := targets()
		if len(games) == 0 {
			return
		}
		// The button is disabled for a single game GOG has no files for, but
		// Enter on the list reaches here too, so the same case is refused.
		if len(games) == 1 && sel.count() == 0 && !shownDownloadable {
			dialog.ShowInformation("Nothing to Download",
				fmt.Sprintf("GOG serves no downloadable files for %s.", games[0].Title), win)
			return
		}

		result, err := queue(games)
		if err != nil {
			showErrorDialog(win, "Could not start the download", err)
			return
		}
		if len(games) == 1 && result.Queued == 1 {
			dialog.ShowInformation("Started", fmt.Sprintf("Download for '%s' has started.", games[0].Title), win)
			return
		}
		dialog.ShowInformation("Downloads", result.summary(), win)
	}
	downloadBtn = widget.NewButtonWithIcon("Download", theme.DownloadIcon(), startDownload)
	downloadBtn.Importance = widget.HighImportance
	refreshDownloadButton()

	form := widget.NewForm(
		widget.NewFormItem("Download Path", pathContainer),
		widget.NewFormItem("Platforms", platformGroup),
		widget.NewFormItem("Languages", container.NewHScroll(langGrid)),
		widget.NewFormItem("Threads", threadsSelect),
		widget.NewFormItem("Connections", connectionsSelect),
	)
	// Seven switches in a grid say nothing about what they do to each other.
	// Grouped, each heading answers that.
	checkboxes := container.NewVBox(
		optionGroup("What to download", extrasCheck, dlcsCheck),
		optionGroup("Which files", resumeCheck, skipPatchesCheck, keepLatestCheck),
		optionGroup("Where they go", flattenCheck, rommCheck, lutrisCheck),
	)

	relabel := refreshDownloadButton

	return &downloadForm{
		options: container.NewVBox(form, widget.NewSeparator(), checkboxes),
		// One row. The ways out to the web sit left, and the things that act
		// on the game sit right: the size estimate, the save backup, and the
		// download, each labeled so nothing has to be hovered to be
		// understood.
		actions: container.NewHBox(
			fixedSize(storeBtn, paneCompactSize), fixedSize(gogdbBtn, paneCompactSize),
			layout.NewSpacer(),
			estimateBtn, savesBtn, fixedSize(downloadBtn, paneActionSize)),
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
func fillDetails(pane *detailsPane, s *libraryState, game db.Game, dm *DownloadManager, meta *client.GameMetadata) {
	details := gameDetails(s, game, dm, meta)

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
	paneActionSize = fyne.NewSize(144, 36)
	// paneCompactSize fits a short word: the web links share the action row
	// with the download button, and every point they take is the pane's.
	paneCompactSize = fyne.NewSize(68, 36)
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
