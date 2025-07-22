package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func SettingsTabUI() fyne.CanvasObject {
	themeRadio := widget.NewRadioGroup([]string{"Light", "Dark"}, func(selected string) {
		a := fyne.CurrentApp()
		if selected == "Light" {
			a.Preferences().SetString("theme", "light")
			a.Settings().SetTheme(theme.LightTheme())
		} else {
			a.Preferences().SetString("theme", "dark")
			a.Settings().SetTheme(NewCustomDarkTheme())
		}
	})
	if fyne.CurrentApp().Preferences().String("theme") == "dark" {
		themeRadio.SetSelected("Dark")
	} else {
		themeRadio.SetSelected("Light")
	}

	themeBox := container.NewHBox(widget.NewLabel("UI Theme"), themeRadio)

	settingsCard := widget.NewCard("UI Configuration", "", container.NewVBox(themeBox))

	return container.NewCenter(settingsCard)
}
