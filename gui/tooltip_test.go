package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/require"
)

// tipWindow is a window whose content sits under a tip layer, the way the
// real window is assembled.
func tipWindow(t *testing.T, content fyne.CanvasObject) (fyne.Window, *fyne.Container) {
	t.Helper()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)
	win.SetContent(installTipLayer(win.Canvas(), content))
	t.Cleanup(func() { removeTipLayer(win.Canvas()) })
	win.Resize(fyne.NewSize(400, 300))
	return win, tipLayerFor(win.Canvas())
}

// The tip is drawn on the layer, never as an overlay: an overlay swallows
// the hover that keeps the tip alive, and the tip then flickers. This is the
// regression test for that flicker.
func TestIconButtonTip_NeverOpensAnOverlay(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	button := newIconButton(theme.InfoIcon(), "Estimate the download size", nil)
	win, layer := tipWindow(t, button)

	button.MouseIn(&desktop.MouseEvent{})
	require.Len(t, layer.Objects, 1, "the tip sits on the layer")
	require.Empty(t, win.Canvas().Overlays().List(),
		"an overlay would take the hover away and flicker the tip")

	// The hover events keep coming while the pointer rests; the tip must
	// neither duplicate nor blink.
	button.MouseIn(&desktop.MouseEvent{})
	require.Len(t, layer.Objects, 1, "one pointer, one tip")

	button.MouseOut()
	require.Empty(t, layer.Objects, "the tip leaves with the pointer")
}

// A window whose content was never wrapped has nowhere to draw, and hovering
// must not panic in that world; the fixtures build bare trees.
func TestIconButtonTip_SurvivesWithoutALayer(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	button := newIconButton(theme.InfoIcon(), "Anything", nil)
	win := test.NewWindow(button)
	t.Cleanup(win.Close)

	button.MouseIn(&desktop.MouseEvent{})
	require.Nil(t, button.shown)
	button.MouseOut()
}

// The tip stays inside the window instead of running off its right edge.
func TestShowTip_StaysInsideTheWindow(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	button := newIconButton(theme.InfoIcon(), "A tip with enough words to be wide", nil)
	win, layer := tipWindow(t, button)

	// The button hugs the right edge, where a naive tip would overflow.
	button.Move(fyne.NewPos(win.Canvas().Size().Width-button.MinSize().Width, 0))
	button.MouseIn(&desktop.MouseEvent{})
	require.Len(t, layer.Objects, 1)

	tip := layer.Objects[0]
	require.LessOrEqual(t, tip.Position().X+tip.Size().Width, win.Canvas().Size().Width,
		"the tip must not leave the window")
}
