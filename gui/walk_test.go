package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// walkWidgets visits every object in a widget tree, descending through the
// containers and widgets that hold children. Anything that can nest in this
// app belongs here, so a new wrapper does not quietly hide widgets from tests.
func walkWidgets(o fyne.CanvasObject, visit func(fyne.CanvasObject)) {
	if o == nil {
		return
	}
	visit(o)

	switch v := o.(type) {
	case *fyne.Container:
		for _, child := range v.Objects {
			walkWidgets(child, visit)
		}
	case *container.Scroll:
		walkWidgets(v.Content, visit)
	case *container.Split:
		walkWidgets(v.Leading, visit)
		walkWidgets(v.Trailing, visit)
	case *widget.Card:
		walkWidgets(v.Content, visit)
	case *widget.Form:
		for _, item := range v.Items {
			walkWidgets(item.Widget, visit)
		}
	case *widget.Accordion:
		for _, item := range v.Items {
			walkWidgets(item.Detail, visit)
		}
	}
}

// widgetsOfType returns every widget of type T in a tree.
func widgetsOfType[T any](root fyne.CanvasObject) []T {
	var found []T
	walkWidgets(root, func(o fyne.CanvasObject) {
		if typed, ok := o.(T); ok {
			found = append(found, typed)
		}
	})
	return found
}

func buttonWithLabel(root fyne.CanvasObject, label string) *widget.Button {
	for _, button := range widgetsOfType[*widget.Button](root) {
		if button.Text == label {
			return button
		}
	}
	return nil
}

func selectWithOption(t *testing.T, root fyne.CanvasObject, option string) *widget.Select {
	t.Helper()
	for _, sel := range widgetsOfType[*widget.Select](root) {
		for _, candidate := range sel.Options {
			if candidate == option {
				return sel
			}
		}
	}
	t.Fatalf("no select offering %q", option)
	return nil
}
