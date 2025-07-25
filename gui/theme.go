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
	switch name {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 0x28, G: 0x28, B: 0x28, A: 0xff}
	case theme.ColorNameButton:
		return color.NRGBA{R: 0x3a, G: 0x3a, B: 0x3a, A: 0xff}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xff}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 0x44, G: 0x44, B: 0x44, A: 0xff}
	case theme.ColorNameHover:
		return color.NRGBA{R: 0x50, G: 0x50, B: 0x50, A: 0xff}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 0x33, G: 0x33, B: 0x33, A: 0xff}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 0x66, G: 0x66, B: 0x66, A: 0xff}
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0x60, G: 0x60, B: 0x60, A: 0xff}
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x80}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0x33, G: 0x99, B: 0xff, A: 0xff}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t *customDarkTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *customDarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *customDarkTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
