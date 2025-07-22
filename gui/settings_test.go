package gui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/gui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsTabUI_Render(t *testing.T) {
	test.NewApp()
	ui := gui.SettingsTabUI()
	assert.NotNil(t, ui)

	card := test.FindObject(ui, func(o fyne.CanvasObject) bool {
		c, ok := o.(*widget.Card)
		return ok && c.Title == "UI Configuration"
	}).(*widget.Card)
	assert.NotNil(t, card, "should find a card with the title 'UI Configuration'")
}

func TestSettingsTabUI_ThemeSwitching(t *testing.T) {
	a := test.NewApp()
	a.Preferences().SetString("theme", "light") // Start with a known state

	w := test.NewWindow(gui.SettingsTabUI())
	defer w.Close()

	var radio *widget.RadioGroup
	test.FindObject(w.Content(), func(o fyne.CanvasObject) bool {
		r, ok := o.(*widget.RadioGroup)
		if ok {
			radio = r
			return true
		}
		return false
	})
	require.NotNil(t, radio, "should find the radio group widget")

	// 1. Verify initial state is Light
	assert.Equal(t, "Light", radio.Selected)
	assert.Equal(t, theme.LightTheme(), a.Settings().Theme())
	assert.Equal(t, "light", a.Preferences().String("theme"))

	// 2. Switch to Dark
	radio.SetSelected("Dark")

	assert.Equal(t, "Dark", radio.Selected)
	assert.Equal(t, "dark", a.Preferences().String("theme"))
	// Compare a known unique property of our custom theme
	expectedBg := gui.NewCustomDarkTheme().Color(theme.ColorNameBackground, theme.VariantDark)
	actualBg := a.Settings().Theme().Color(theme.ColorNameBackground, theme.VariantDark)
	assert.Equal(t, expectedBg, actualBg, "app theme should be custom dark theme")

	// 3. Switch back to Light
	radio.SetSelected("Light")

	assert.Equal(t, "Light", radio.Selected)
	assert.Equal(t, "light", a.Preferences().String("theme"))
	assert.Equal(t, theme.LightTheme(), a.Settings().Theme())
}
