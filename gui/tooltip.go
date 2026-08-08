package gui

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Tips are drawn on a transparent layer stacked over the window's content,
// never as popup overlays: an overlay swallows hover events, so a popup tip
// used to take the hover away from its own button the moment it appeared,
// which hid the tip, which gave the hover back, which showed it again. The
// layer draws without taking anything.

// tipLayers is the canvas-to-layer lookup, filled by installTipLayer when a
// window's content is assembled. It is a binding between a window and its
// one tip surface rather than mutable program state.
var tipLayers sync.Map

// installTipLayer stacks a tip surface over a window's content and registers
// it for the buttons drawn on that canvas. The caller hands the result to
// SetContent.
func installTipLayer(cnv fyne.Canvas, content fyne.CanvasObject) fyne.CanvasObject {
	layer := container.NewWithoutLayout()
	tipLayers.Store(cnv, layer)
	return container.NewStack(content, layer)
}

// removeTipLayer forgets a window's tip surface. Tests install layers per
// window and clean up after themselves.
func removeTipLayer(cnv fyne.Canvas) {
	tipLayers.Delete(cnv)
}

// tipLayerFor is the tip surface of the canvas a button sits on, or nil when
// none was installed, in which case no tip is shown.
func tipLayerFor(cnv fyne.Canvas) *fyne.Container {
	if cnv == nil {
		return nil
	}
	layer, ok := tipLayers.Load(cnv)
	if !ok {
		return nil
	}
	return layer.(*fyne.Container)
}

// newTipBubble is the tip itself: the words on a rounded card that stands
// out from whatever is behind it.
func newTipBubble(text string) fyne.CanvasObject {
	background := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	background.StrokeColor = theme.Color(theme.ColorNameSeparator)
	background.StrokeWidth = 1
	background.CornerRadius = 6
	return container.NewStack(background, widget.NewLabel(text))
}

// showTip places a bubble under an object on its canvas's tip layer, kept
// inside the window's right edge. It reports what it showed, or nil when the
// canvas has no layer to draw on.
func showTip(object fyne.CanvasObject, text string) fyne.CanvasObject {
	driver := fyne.CurrentApp().Driver()
	cnv := driver.CanvasForObject(object)
	layer := tipLayerFor(cnv)
	if layer == nil {
		return nil
	}

	tip := newTipBubble(text)
	size := tip.MinSize()
	position := driver.AbsolutePositionForObject(object).
		Add(fyne.NewPos(0, object.Size().Height+2))
	if farthest := cnv.Size().Width - size.Width - 2; position.X > farthest {
		position.X = farthest
	}
	tip.Resize(size)
	tip.Move(position)
	layer.Add(tip)
	layer.Refresh()
	return tip
}

// hideTip takes a bubble off the layer it was shown on.
func hideTip(object fyne.CanvasObject, tip fyne.CanvasObject) {
	layer := tipLayerFor(fyne.CurrentApp().Driver().CanvasForObject(object))
	if layer == nil || tip == nil {
		return
	}
	layer.Remove(tip)
	layer.Refresh()
}
