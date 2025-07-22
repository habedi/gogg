package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type customDarkTheme struct{}

func NewCustomDarkTheme() fyne.Theme {
	return &customDarkTheme{}
}

func (t *customDarkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameBackground {
		return color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff}
	}

	return theme.DarkTheme().Color(name, variant)
}

func (t *customDarkTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DarkTheme().Font(style)
}

func (t *customDarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DarkTheme().Icon(name)
}

func (t *customDarkTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DarkTheme().Size(name)
}
