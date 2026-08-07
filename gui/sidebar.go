package gui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/search"
)

// sidebarWidth is what the collections get when they are shown. The widest
// name, "Not downloaded", measures 116 with its icon and a four-figure count
// either side of it, so there is room left for names yet to come without
// taking what the details pane needs to fit the default window with margin
// to spare.
const sidebarWidth = 220

// prefSidebar remembers whether the collections are shown. They are not, until
// asked for: most of the time the list is what the window is for.
const prefSidebar = "library.sidebar"

// sidebarRow is one collection. It is nothing but a stored query, which is what
// makes a row and a typed search the same thing.
type sidebarRow struct {
	Title string
	Icon  fyne.Resource
	Query string
	// Header starts a group; a header has no query of its own.
	Header bool
	// HideWhenEmpty keeps a row that has nothing in it out of the way, rather
	// than offering a collection that would come back empty.
	HideWhenEmpty bool
}

// libraryCollections are the ways of looking at a library that gogg offers
// without being asked.
func libraryCollections() []sidebarRow {
	return []sidebarRow{
		{Title: "Library", Header: true},
		{Title: "All games", Icon: theme.StorageIcon(), Query: ""},
		{Title: "Downloaded", Icon: theme.ConfirmIcon(), Query: "downloaded:yes"},
		{Title: "Not downloaded", Icon: theme.DownloadIcon(), Query: "downloaded:no"},
		{Title: "Updates", Icon: theme.ViewRefreshIcon(), Query: "updates:yes", HideWhenEmpty: true},
		{Title: "Favorites", Icon: iconStarOutline, Query: "favorite:yes", HideWhenEmpty: true},
		{Title: "Hidden", Icon: theme.VisibilityOffIcon(), Query: "hidden:yes", HideWhenEmpty: true},

		{Title: "Platform", Header: true},
		{Title: "Windows", Icon: theme.ComputerIcon(), Query: "platform:windows", HideWhenEmpty: true},
		{Title: "macOS", Icon: theme.ComputerIcon(), Query: "platform:mac", HideWhenEmpty: true},
		{Title: "Linux", Icon: theme.ComputerIcon(), Query: "platform:linux", HideWhenEmpty: true},
	}
}

// collectionCounts says how many games each collection holds. Every game is
// described once, with everything any of the rows asks about, so the catalogue
// is parsed once rather than once per row.
func collectionCounts(games []db.Game, rows []sidebarRow) map[string]int {
	queries := make(map[string]search.Query, len(rows))
	var needs search.Needs
	for _, row := range rows {
		if row.Header {
			continue
		}
		query, err := search.Parse(row.Query)
		if err != nil {
			continue
		}
		// Counted the way the list lists: a hidden game is not among "all
		// games" any more than it is in the list.
		query = withoutHidden(query)
		queries[row.Title] = query

		asks := query.Needs()
		needs.Platforms = needs.Platforms || asks.Platforms
		needs.Languages = needs.Languages || asks.Languages
		needs.Size = needs.Size || asks.Size
		needs.Tags = needs.Tags || asks.Tags
	}

	counts := make(map[string]int, len(queries))
	for _, game := range games {
		facts := factsFor(game, needs)
		for title, query := range queries {
			if query.Match(facts) {
				counts[title]++
			}
		}
	}
	return counts
}

// sidebarGroup is a heading and the collections listed under it. A heading with
// everything under it hidden describes a group that is not there, so it is
// hidden with them.
type sidebarGroup struct {
	heading *widget.Label
	rows    []string
}

// anyVisible reports whether the group still has a collection to head.
func (g sidebarGroup) anyVisible(buttons map[string]*sidebarButton) bool {
	for _, title := range g.rows {
		if button, ok := buttons[title]; ok && button.Visible() {
			return true
		}
	}
	return false
}

// librarySidebar lists the collections down the side of the library.
type librarySidebar struct {
	content *fyne.Container
	rows    []sidebarRow
	buttons map[string]*sidebarButton
	groups  []sidebarGroup
	// refresh recounts the collections and hides the empty ones.
	refresh func(games []db.Game)
	// syncTo marks the row that matches what the search box says, or none.
	syncTo func(searchText string)
}

// newLibrarySidebar builds the collections. Picking one is the same as typing
// its query, so onPick is handed the query rather than the row.
func newLibrarySidebar(rows []sidebarRow, onPick func(query string)) *librarySidebar {
	sidebar := &librarySidebar{rows: rows, buttons: make(map[string]*sidebarButton, len(rows))}

	items := make([]fyne.CanvasObject, 0, len(rows))
	for _, row := range rows {
		if row.Header {
			heading := widget.NewLabel(strings.ToUpper(row.Title))
			heading.TextStyle = fyne.TextStyle{Bold: true}
			items = append(items, heading)
			sidebar.groups = append(sidebar.groups, sidebarGroup{heading: heading})
			continue
		}

		picked := row
		button := newSidebarButton(row, func() { onPick(picked.Query) })
		sidebar.buttons[row.Title] = button
		if last := len(sidebar.groups) - 1; last >= 0 {
			sidebar.groups[last].rows = append(sidebar.groups[last].rows, row.Title)
		}
		items = append(items, button)
	}

	// The list beside it is what people read, so the collections keep to their
	// own width rather than taking half the window.
	column := container.NewVScroll(container.NewVBox(items...))
	column.SetMinSize(fyne.NewSize(sidebarWidth, 0))
	sidebar.content = container.NewStack(column)

	sidebar.refresh = func(games []db.Game) {
		counts := collectionCounts(games, rows)
		for _, row := range rows {
			button, ok := sidebar.buttons[row.Title]
			if !ok {
				continue
			}
			count := counts[row.Title]
			button.setCount(count)
			if row.HideWhenEmpty && count == 0 {
				button.Hide()
				continue
			}
			button.Show()
		}

		for _, group := range sidebar.groups {
			if group.anyVisible(sidebar.buttons) {
				group.heading.Show()
				continue
			}
			group.heading.Hide()
		}
	}

	sidebar.syncTo = func(searchText string) {
		terms := strings.TrimSpace(strings.TrimPrefix(searchText, search.Words(searchText)))
		for _, row := range rows {
			button, ok := sidebar.buttons[row.Title]
			if !ok {
				continue
			}
			button.setSelected(strings.EqualFold(terms, row.Query))
		}
	}

	sidebar.syncTo("")
	return sidebar
}

// sidebarButton is one collection: an icon, a name, and how much is in it.
type sidebarButton struct {
	widget.BaseWidget

	icon  *widget.Icon
	title *widget.Label
	count *widget.Label
	mark  *canvas.Rectangle
	onTap func()
}

func newSidebarButton(row sidebarRow, onTap func()) *sidebarButton {
	title := widget.NewLabel(row.Title)
	title.Truncation = fyne.TextTruncateEllipsis

	count := widget.NewLabel("")
	count.Alignment = fyne.TextAlignTrailing

	mark := canvas.NewRectangle(theme.Color(theme.ColorNameHover))
	mark.CornerRadius = theme.Padding()
	mark.FillColor = nil

	button := &sidebarButton{
		icon: widget.NewIcon(row.Icon), title: title, count: count, mark: mark, onTap: onTap,
	}
	button.ExtendBaseWidget(button)
	return button
}

func (b *sidebarButton) CreateRenderer() fyne.WidgetRenderer {
	row := container.NewBorder(nil, nil,
		container.New(layout.NewGridWrapLayout(fyne.NewSize(theme.IconInlineSize(), theme.IconInlineSize())), b.icon),
		b.count, b.title)
	return widget.NewSimpleRenderer(container.NewStack(b.mark, row))
}

func (b *sidebarButton) Tapped(_ *fyne.PointEvent) {
	if b.onTap != nil {
		b.onTap()
	}
}

func (b *sidebarButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

// setCount says how much is in a collection. A zero is written out rather than
// left blank: "Downloaded 0" answers the question, an empty space raises it.
func (b *sidebarButton) setCount(count int) {
	b.count.SetText(fmt.Sprintf("%d", count))
}

func (b *sidebarButton) setSelected(selected bool) {
	if selected {
		b.mark.FillColor = theme.Color(theme.ColorNameSelection)
		b.title.TextStyle = fyne.TextStyle{Bold: true}
	} else {
		b.mark.FillColor = nil
		b.title.TextStyle = fyne.TextStyle{}
	}
	b.mark.Refresh()
	b.title.Refresh()
}
