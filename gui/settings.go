package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func SettingsTabUI(win fyne.Window) fyne.CanvasObject {
	// --- Theme Settings ---
	themeRadio := widget.NewRadioGroup([]string{"Light", "Dark"}, func(selected string) {
		a := fyne.CurrentApp()
		if selected == "Light" {
			a.Preferences().SetString("theme", "light")
			a.Settings().SetTheme(theme.DefaultTheme())
		} else {
			a.Preferences().SetString("theme", "dark")
			a.Settings().SetTheme(NewCustomDarkTheme())
		}
	})
	if fyne.CurrentApp().Preferences().StringWithFallback("theme", "light") == "dark" {
		themeRadio.SetSelected("Dark")
	} else {
		themeRadio.SetSelected("Light")
	}
	themeBox := container.NewHBox(widget.NewLabel("UI Theme"), themeRadio)

	// --- Sound Settings ---
	soundCheck := widget.NewCheck("Play sound on download completion", func(checked bool) {
		fyne.CurrentApp().Preferences().SetBool("soundEnabled", checked)
	})
	soundCheck.SetChecked(fyne.CurrentApp().Preferences().BoolWithFallback("soundEnabled", true))

	soundPathLabel := widget.NewLabel(fyne.CurrentApp().Preferences().String("soundFilePath"))
	if soundPathLabel.Text == "" {
		soundPathLabel.SetText("Default")
	}

	selectSoundBtn := widget.NewButton("Select Custom Sound...", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, win)
				return
			}
			if reader == nil {
				return
			}
			path := reader.URI().Path()
			fyne.CurrentApp().Preferences().SetString("soundFilePath", path)
			soundPathLabel.SetText(path)
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp3", ".ogg", ".aac"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})

	resetSoundBtn := widget.NewButton("Reset", func() {
		fyne.CurrentApp().Preferences().RemoveValue("soundFilePath")
		soundPathLabel.SetText("Default")
	})

	testSoundBtn := widget.NewButton("Test", func() {
		go PlayNotificationSound()
	})

	soundConfigBox := container.NewVBox(
		widget.NewLabel("Current sound file:"),
		soundPathLabel,
		container.NewHBox(selectSoundBtn, resetSoundBtn, testSoundBtn),
	)

	// --- Layout ---
	mainCard := widget.NewCard("Settings", "", container.NewVBox(
		themeBox,
		widget.NewSeparator(),
		soundCheck,
		soundConfigBox,
	))

	return container.NewCenter(mainCard)
}
