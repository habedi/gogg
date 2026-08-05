package gui

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// prefXMB is set when the user asks for the cross interface instead of the tabs.
const prefXMB = "ui.xmb"

const (
	// xmbCategoryWidth is the room one category gets, and so how far apart the
	// icons sit along the bar.
	xmbCategoryWidth = 96
	// xmbCategoryHeight leaves room for an icon with its title underneath.
	xmbCategoryHeight = 84
	// xmbIconSize is the icon itself, which stays the same whether or not the
	// category is the one selected.
	xmbIconSize = 40
)

// xmbItem is one thing listed under a category. It either opens a pane or does
// something on the spot, such as refreshing the catalogue.
type xmbItem struct {
	Title string
	// Detail is read every time the item is drawn, so counts stay current.
	Detail func() string
	// Pane is what the item opens. Items with none run Action instead.
	Pane   func() fyne.CanvasObject
	Action func()
}

// xmbCategory is one icon on the bar, with the items listed under it.
type xmbCategory struct {
	Title string
	Icon  fyne.Resource
	Items []xmbItem
}

// xmbShell is the cross interface: categories along the bar, their items down
// the column below the one selected. The arrow keys move across and down, and
// Enter opens what is selected.
type xmbShell struct {
	widget.BaseWidget

	categories []xmbCategory
	category   int
	item       int

	categoryRow  *fyne.Container
	categoryKeys []*xmbCategoryButton
	indent       *canvas.Rectangle
	itemColumn   *fyne.Container
	itemKeys     []*xmbItemButton

	browse   *fyne.Container
	paneBox  *fyne.Container
	paneOpen bool
	content  *fyne.Container
}

func newXMBShell(categories []xmbCategory) *xmbShell {
	shell := &xmbShell{categories: categories, category: -1}

	shell.categoryRow = container.NewHBox()
	for i, category := range categories {
		index := i
		key := newXMBCategoryButton(category, func() {
			shell.takeFocus()
			shell.selectCategory(index)
		})
		shell.categoryKeys = append(shell.categoryKeys, key)
		shell.categoryRow.Add(key)
	}

	// The column of items hangs under the icon it belongs to, which is what the
	// indent in front of it is for.
	shell.indent = canvas.NewRectangle(color.Transparent)
	shell.itemColumn = container.NewVBox()
	items := container.NewHBox(shell.indent, shell.itemColumn)

	shell.browse = container.NewVBox(
		layout.NewSpacer(),
		shell.categoryRow,
		items,
		layout.NewSpacer(),
	)
	shell.paneBox = container.NewStack()
	shell.paneBox.Hide()

	shell.content = container.NewStack(
		xmbBackground(),
		container.NewPadded(shell.browse),
		shell.paneBox,
	)

	shell.ExtendBaseWidget(shell)
	if len(categories) > 0 {
		shell.selectCategory(0)
	}
	return shell
}

func (s *xmbShell) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(s.content)
}

// selectCategory moves along the bar, which starts its items from the top.
func (s *xmbShell) selectCategory(index int) {
	if index < 0 || index >= len(s.categories) || index == s.category {
		return
	}
	s.category = index
	for i, key := range s.categoryKeys {
		key.setSelected(i == index)
	}

	s.indent.SetMinSize(fyne.NewSize(float32(index)*(xmbCategoryWidth+theme.Padding()), 0))
	s.listItems()
	s.item = -1
	s.selectItem(0)
}

// listItems rebuilds the column under the selected category.
func (s *xmbShell) listItems() {
	s.itemKeys = nil
	s.itemColumn.Objects = nil
	for i, item := range s.categories[s.category].Items {
		index := i
		key := newXMBItemButton(item, func() {
			s.takeFocus()
			s.selectItem(index)
			s.activate()
		})
		s.itemKeys = append(s.itemKeys, key)
		s.itemColumn.Add(key)
	}
	s.itemColumn.Refresh()
}

// selectItem moves down the column.
func (s *xmbShell) selectItem(index int) {
	if index < 0 || index >= len(s.itemKeys) || index == s.item {
		return
	}
	s.item = index
	for i, key := range s.itemKeys {
		key.setSelected(i == index)
	}
}

// activate opens what is selected, or runs it when there is nothing to open.
func (s *xmbShell) activate() {
	items := s.categories[s.category].Items
	if s.item < 0 || s.item >= len(items) {
		return
	}
	item := items[s.item]
	switch {
	case item.Pane != nil:
		s.openPane(item.Title, item.Pane())
	case item.Action != nil:
		item.Action()
	}
}

// openPane gives the whole window over to one pane, with a way back to the bar.
func (s *xmbShell) openPane(title string, pane fyne.CanvasObject) {
	if pane == nil {
		return
	}
	back := widget.NewButtonWithIcon("Back", theme.NavigateBackIcon(), s.closePane)
	heading := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	head := container.NewBorder(nil, nil, back, nil, heading)

	s.paneBox.Objects = []fyne.CanvasObject{container.NewBorder(head, nil, nil, nil, pane)}
	s.paneBox.Show()
	s.paneOpen = true
	s.browse.Hide()
	s.Refresh()
}

// closePane returns to the bar.
func (s *xmbShell) closePane() {
	s.paneBox.Objects = nil
	s.paneBox.Hide()
	s.paneOpen = false
	s.browse.Show()
	s.Refresh()
	s.takeFocus()
}

func (s *xmbShell) takeFocus() {
	if canvas := fyne.CurrentApp().Driver().CanvasForObject(s); canvas != nil {
		canvas.Focus(s)
	}
}

// TypedKey is the cross: left and right along the bar, up and down the column,
// Enter to open, and Escape to come back from a pane.
func (s *xmbShell) TypedKey(event *fyne.KeyEvent) {
	if s.paneOpen {
		if event.Name == fyne.KeyEscape {
			s.closePane()
		}
		return
	}

	switch event.Name {
	case fyne.KeyLeft:
		s.selectCategory(s.category - 1)
	case fyne.KeyRight:
		s.selectCategory(s.category + 1)
	case fyne.KeyUp:
		s.selectItem(s.item - 1)
	case fyne.KeyDown:
		s.selectItem(s.item + 1)
	case fyne.KeyReturn, fyne.KeyEnter, fyne.KeySpace:
		s.activate()
	}
}

func (s *xmbShell) TypedRune(_ rune) {}
func (s *xmbShell) FocusGained()     {}
func (s *xmbShell) FocusLost()       {}
func (s *xmbShell) Tapped(_ *fyne.PointEvent) {
	s.takeFocus()
}

// xmbCategoryButton is one icon on the bar.
type xmbCategoryButton struct {
	widget.BaseWidget

	icon  *widget.Icon
	title *widget.Label
	mark  *canvas.Rectangle
	onTap func()
}

func newXMBCategoryButton(category xmbCategory, onTap func()) *xmbCategoryButton {
	icon := widget.NewIcon(category.Icon)
	title := widget.NewLabelWithStyle(category.Title, fyne.TextAlignCenter, fyne.TextStyle{})

	mark := canvas.NewRectangle(color.Transparent)
	mark.StrokeColor = theme.Color(theme.ColorNamePrimary)
	mark.StrokeWidth = 0
	mark.CornerRadius = theme.Padding()

	key := &xmbCategoryButton{icon: icon, title: title, mark: mark, onTap: onTap}
	key.ExtendBaseWidget(key)
	return key
}

func (c *xmbCategoryButton) CreateRenderer() fyne.WidgetRenderer {
	sized := container.New(layout.NewGridWrapLayout(fyne.NewSize(xmbIconSize, xmbIconSize)), c.icon)
	stacked := container.NewStack(c.mark,
		container.NewVBox(container.NewCenter(sized), c.title))
	return widget.NewSimpleRenderer(stacked)
}

// MinSize keeps every category the same width, so the column below the bar can
// be lined up under the one selected.
func (c *xmbCategoryButton) MinSize() fyne.Size {
	return fyne.NewSize(xmbCategoryWidth, xmbCategoryHeight)
}

func (c *xmbCategoryButton) Tapped(_ *fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap()
	}
}

func (c *xmbCategoryButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (c *xmbCategoryButton) setSelected(selected bool) {
	if selected {
		c.mark.StrokeWidth = 2
		c.title.TextStyle = fyne.TextStyle{Bold: true}
	} else {
		c.mark.StrokeWidth = 0
		c.title.TextStyle = fyne.TextStyle{}
	}
	c.mark.Refresh()
	c.title.Refresh()
}

// xmbItemButton is one entry in the column under a category.
type xmbItemButton struct {
	widget.BaseWidget

	marker *widget.Label
	title  *widget.Label
	detail *widget.Label
	onTap  func()
}

func newXMBItemButton(item xmbItem, onTap func()) *xmbItemButton {
	detail := ""
	if item.Detail != nil {
		detail = item.Detail()
	}

	key := &xmbItemButton{
		marker: widget.NewLabel(" "),
		title:  widget.NewLabel(item.Title),
		detail: widget.NewLabel(detail),
		onTap:  onTap,
	}
	key.ExtendBaseWidget(key)
	return key
}

func (i *xmbItemButton) CreateRenderer() fyne.WidgetRenderer {
	row := container.NewHBox(i.marker, i.title, i.detail)
	return widget.NewSimpleRenderer(row)
}

func (i *xmbItemButton) Tapped(_ *fyne.PointEvent) {
	if i.onTap != nil {
		i.onTap()
	}
}

func (i *xmbItemButton) Cursor() desktop.Cursor { return desktop.PointerCursor }

func (i *xmbItemButton) setSelected(selected bool) {
	if selected {
		i.marker.SetText("▸")
		i.title.TextStyle = fyne.TextStyle{Bold: true}
	} else {
		i.marker.SetText(" ")
		i.title.TextStyle = fyne.TextStyle{}
	}
	i.title.Refresh()
}

// xmbBackground is the dark gradient with a band of light across it. The cross
// interface is dark whatever theme is set, which is the look it comes from.
func xmbBackground() fyne.CanvasObject {
	gradient := canvas.NewVerticalGradient(
		color.NRGBA{R: 0x0b, G: 0x1b, B: 0x30, A: 0xff},
		color.NRGBA{R: 0x03, G: 0x07, B: 0x0f, A: 0xff},
	)

	wave := canvas.NewImageFromImage(xmbWave(256, 128))
	wave.FillMode = canvas.ImageFillStretch
	wave.Translucency = 0.35

	return container.NewStack(gradient, wave)
}

// xmbWave draws a band that rises and falls across the picture, faded out above
// and below it.
func xmbWave(width, height int) image.Image {
	band := image.NewNRGBA(image.Rect(0, 0, width, height))
	thickness := float64(height) / 6

	for x := 0; x < width; x++ {
		phase := float64(x) / float64(width) * 2 * math.Pi
		centre := float64(height)/2 + math.Sin(phase)*float64(height)/6

		for y := 0; y < height; y++ {
			away := math.Abs(float64(y) - centre)
			if away > thickness {
				continue
			}
			strength := 1 - away/thickness
			band.SetNRGBA(x, y, color.NRGBA{
				R: 0x4c, G: 0x9a, B: 0xd4, A: uint8(strength * 0xa0),
			})
		}
	}
	return band
}
