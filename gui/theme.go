package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// forcedTheme is a simple wrapper around a theme that forces a specific variant.
type forcedTheme struct {
	fyne.Theme
	variant fyne.ThemeVariant
}

// NewForcedTheme creates a new theme that forces a light or dark variant
// while inheriting all other properties from the default theme.
func NewForcedTheme(variant fyne.ThemeVariant) fyne.Theme {
	return &forcedTheme{
		Theme:   theme.DefaultTheme(),
		variant: variant,
	}
}

// Color overrides the Color method to ignore the incoming variant from the system
// and use the forced variant we have stored.
func (t *forcedTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	return t.Theme.Color(name, t.variant)
}
