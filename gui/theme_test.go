package gui_test

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/habedi/gogg/gui"
	"github.com/stretchr/testify/assert"
)

func TestCustomDarkTheme(t *testing.T) {
	customTheme := gui.NewCustomDarkTheme()
	defaultDarkTheme := theme.DarkTheme()

	t.Run("Color", func(t *testing.T) {
		expectedBg := color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff}
		actualBg := customTheme.Color(theme.ColorNameBackground, theme.VariantDark)
		assert.Equal(t, expectedBg, actualBg, "should have a custom background color")

		expectedPrimary := defaultDarkTheme.Color(theme.ColorNamePrimary, theme.VariantDark)
		actualPrimary := customTheme.Color(theme.ColorNamePrimary, theme.VariantDark)
		assert.Equal(t, expectedPrimary, actualPrimary, "should delegate other colors to the default dark theme")
	})

	t.Run("Font", func(t *testing.T) {
		expectedFont := defaultDarkTheme.Font(fyne.TextStyle{Bold: true})
		actualFont := customTheme.Font(fyne.TextStyle{Bold: true})
		assert.Equal(t, expectedFont, actualFont, "should delegate fonts to the default dark theme")
	})

	t.Run("Icon", func(t *testing.T) {
		expectedIcon := defaultDarkTheme.Icon(theme.IconNameHome)
		actualIcon := customTheme.Icon(theme.IconNameHome)
		assert.Equal(t, expectedIcon, actualIcon, "should delegate icons to the default dark theme")
	})

	t.Run("Size", func(t *testing.T) {
		expectedSize := defaultDarkTheme.Size(theme.SizeNamePadding)
		actualSize := customTheme.Size(theme.SizeNamePadding)
		assert.Equal(t, expectedSize, actualSize, "should delegate sizes to the default dark theme")
	})
}
