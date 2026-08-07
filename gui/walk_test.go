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
	case *widget.PopUp:
		// Dialogs and menus are shown in an overlay of their own, so a test
		// asking what a dialog says has to be able to look inside one.
		walkWidgets(v.Content, visit)
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
	case *factsGrid:
		for _, half := range v.halves {
			walkWidgets(half, visit)
		}
	case *pictureViewer:
		walkWidgets(v.content, visit)
	case *container.AppTabs:
		// Every tab, not only the one on show: a test asking what the pane holds
		// should not have to click through it.
		for _, item := range v.Items {
			walkWidgets(item.Content, visit)
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

// iconButtonWithTip finds an icon button by what it tells the user on hover,
// which is the only words an icon button has.
func iconButtonWithTip(root fyne.CanvasObject, tip string) *iconButton {
	for _, button := range widgetsOfType[*iconButton](root) {
		if button.tip == tip {
			return button
		}
	}
	return nil
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

// labelTexts is what a tree says, in plain labels and copyable ones alike.
func labelTexts(root fyne.CanvasObject) []string {
	var texts []string
	for _, label := range widgetsOfType[*widget.Label](root) {
		texts = append(texts, label.Text)
	}
	for _, label := range widgetsOfType[*CopyableLabel](root) {
		texts = append(texts, label.Text)
	}
	return texts
}

// checkWithLabel finds a check box by what it says.
func checkWithLabel(root fyne.CanvasObject, label string) *widget.Check {
	for _, check := range widgetsOfType[*widget.Check](root) {
		if check.Text == label {
			return check
		}
	}
	return nil
}
