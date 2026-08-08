package gui

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// checkGrid is a set of check boxes laid out in a fixed number of columns. It
// carries the same idea as widget.CheckGroup, which only offers one column or
// one row, and is for choices too many to want as one tall column, such as
// the download languages.
type checkGrid struct {
	widget.BaseWidget

	columns   int
	options   []string
	checks    []*widget.Check
	onChanged func([]string)
	grid      *fyne.Container
}

func newCheckGrid(columns int) *checkGrid {
	g := &checkGrid{columns: columns, grid: container.NewGridWithColumns(columns)}
	g.ExtendBaseWidget(g)
	return g
}

func (g *checkGrid) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(g.grid)
}

// setOptions rebuilds the boxes, keeping any current selection that is still
// on offer.
func (g *checkGrid) setOptions(options []string) {
	kept := make(map[string]bool, len(g.checks))
	for _, name := range g.selected() {
		kept[name] = true
	}

	g.options = append([]string{}, options...)
	g.checks = make([]*widget.Check, 0, len(options))
	cells := make([]fyne.CanvasObject, 0, len(options))
	for _, option := range options {
		check := widget.NewCheck(option, func(bool) { g.emit() })
		if kept[option] {
			check.SetChecked(true)
		}
		g.checks = append(g.checks, check)
		cells = append(cells, check)
	}
	g.grid.Objects = cells
	g.grid.Refresh()
}

// selected is the ticked boxes, in the order they are offered.
func (g *checkGrid) selected() []string {
	chosen := make([]string, 0, len(g.checks))
	for _, check := range g.checks {
		if check.Checked {
			chosen = append(chosen, check.Text)
		}
	}
	return chosen
}

// setSelected ticks exactly the given boxes, leaving the rest clear.
func (g *checkGrid) setSelected(selected []string) {
	want := make(map[string]bool, len(selected))
	for _, name := range selected {
		want[name] = true
	}
	for _, check := range g.checks {
		check.SetChecked(want[check.Text])
	}
}

func (g *checkGrid) emit() {
	if g.onChanged != nil {
		g.onChanged(g.selected())
	}
}

// bindCheckGrid points a check grid at a new set of options, keeping of the
// wanted values the ones on offer. The change handler is detached first, the
// way bindCheckGroup does it: narrowing the boxes to suit a game is not the
// user choosing, and must not be reported as though it were. When nothing
// wanted is on offer, the first box is ticked, since a download needs at
// least one.
func bindCheckGrid(g *checkGrid, options, wanted []string, onChanged func([]string)) {
	g.onChanged = nil
	g.setOptions(options)

	kept := make([]string, 0, len(wanted))
	for _, want := range wanted {
		if slices.Contains(options, want) {
			kept = append(kept, want)
		}
	}
	if len(kept) == 0 && len(options) > 0 {
		kept = options[:1]
	}
	g.setSelected(kept)

	g.onChanged = onChanged
	g.Refresh()
}
