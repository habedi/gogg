package gui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

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
	"github.com/habedi/gogg/pkg/search"
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
	// state is what the library knows about its games beyond the catalogue.
	state *libraryState
	// close detaches what the library listens to. The catalogue signal belongs
	// to the whole app, so a library that has been replaced has to stop
	// following it or it goes on working for a window that is gone.
	close func()
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

// LibraryTabUI builds the catalogue tab. onLogin is invoked when a signed-out
// user asks to log in.
func LibraryTabUI(win fyne.Window, authService *auth.Service, dm *DownloadManager, st stores, onLogin func()) *libraryTab {
	token, _ := st.tokens.Get(context.Background())
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
			state:       newLibraryState(),
			close:       func() {},
		}
	}

	allGames, _ := st.games.List(context.Background())
	state := newLibraryState()
	state.loadTags(st.tags)
	state.loadGenres(st.metadata)
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
	metadata := newMetadataCache(st.metadata)
	metadata.onGenres = func(gameID int, genres []string) { state.genres[gameID] = genres }

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
			if !query.Match(state.factsFor(game, needs)) {
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
				state.applyStatuses(found)
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
			bindGameCell(cell, games[id], rowBinding{sel: sel, covers: covers, dm: dm,
				state: state, win: win, onToggle: func() { afterSelectionChange() }})
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
			bindGameRow(obj, game, rowBinding{sel: sel, covers: covers, dm: dm,
				state: state, win: win, onToggle: func() { afterSelectionChange() }})
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
				state.applyStatuses(found)
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
		pending := state.gamesWithUpdates(allGames)
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
		dialog.ShowInformation("Updates Waiting",
			fmt.Sprintf("%d %s %s new files since being downloaded: %s",
				len(pending), gamesWord(len(pending)), verb, joinTitles(titles)), win)
	}
	onFinishRefresh := func() {
		allGames, _ = st.games.List(context.Background())
		state.loadTags(st.tags)
		state.loadGenres(st.metadata)
		state.forgetParsed()
		recomputeStatuses(announceRefreshOutcome)
		setRefreshEnabled(true)
		showGames()
		// A refresh may have brought games nobody has looked up yet.
		startSweep()
	}
	startRefresh := func() {
		setRefreshEnabled(false)
		refreshCatalogue(win, authService, st.games, onFinishRefresh)
	}

	if len(allGames) == 0 {
		refreshNowBtn = widget.NewButton("Refresh Catalogue", startRefresh)
		refreshNowBtn.Importance = widget.HighImportance
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
	state.initStatusPersistence()
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
			fyne.NewMenuItem("Export Game List as CSV", func() { ExportCatalogueAction(win, st.games, "csv") }),
			fyne.NewMenuItem("Export Full Catalogue as JSON", func() { ExportCatalogueAction(win, st.games, "json") }),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItem("File Hashes...", func() { showFileHashes(win) }),
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
	filtersBtn := newFiltersButton(win, &searchEntry.Entry, updateDisplayedGames)
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
		// Only games there is something to download: ticking one you cannot
		// download is a choice that leads nowhere.
		applyBulkSelection(func() {
			for _, game := range displayedGames() {
				if state.downloadable(game) {
					sel.set(game.ID, true)
				}
			}
		})
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
	sidebar = newLibrarySidebar(libraryCollections(), state, func(query string) {
		text := strings.TrimSpace(search.Words(searchEntry.Text) + " " + query)
		searchEntry.SetText(text)
		updateDisplayedGames()
	})
	leftPane := container.NewBorder(nil, nil, sidebar.content, nil, listPane)

	// Genres arrive one game at a time from the background sweep, and the
	// genre collections count what has arrived: without this, they would
	// stay hidden until something else recounted the sidebar.
	storeGenres := metadata.onGenres
	metadata.onGenres = func(gameID int, genres []string) {
		storeGenres(gameID, genres)
		if sidebar.content.Visible() {
			sidebar.refresh(allGames)
		}
	}

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
	pane := createDetailsPane(win, authService, dm, st, state, selectedGameBinding,
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
		pending := state.gamesWithUpdates(allGames)
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
		pending := state.gamesWithUpdates(allGames)
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
					showErrorDialog(win, "Could not start the downloads", err)
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
		fillDetails(pane, state, game, dm, nil)
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
			fillDetails(pane, state, game, dm, &meta)
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
		state.clearStatuses()
		state.forgetParsed()
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
		state:           state,
		close: func() {
			closed.Store(true)
			metadata.close()
			covers.close()
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
