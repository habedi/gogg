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
	prefs := fyne.CurrentApp().Preferences()
	a := fyne.CurrentApp()

	// --- Theme Settings ---
	themeRadio := widget.NewRadioGroup([]string{"System Default", "Light", "Dark"}, func(selected string) {
		switch selected {
		case "Light":
			prefs.SetString("theme", "light")
			a.Settings().SetTheme(NewForcedTheme(theme.VariantLight))
		case "Dark":
			prefs.SetString("theme", "dark")
			a.Settings().SetTheme(NewForcedTheme(theme.VariantDark))
		default: // "System Default"
			prefs.SetString("theme", "system")
			a.Settings().SetTheme(theme.DefaultTheme())
		}
	})

	// Set the initial selected value for the radio group
	switch prefs.StringWithFallback("theme", "system") {
	case "light":
		themeRadio.SetSelected("Light")
	case "dark":
		themeRadio.SetSelected("Dark")
	default:
		themeRadio.SetSelected("System Default")
	}

	themeBox := container.NewHBox(widget.NewLabel("UI Theme"), themeRadio)

	// --- Sound Settings ---
	soundCheck := widget.NewCheck("Play sound on download completion", func(checked bool) {
		prefs.SetBool("soundEnabled", checked)
	})
	soundCheck.SetChecked(prefs.BoolWithFallback("soundEnabled", true))

	soundPathLabel := widget.NewLabel(prefs.String("soundFilePath"))
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
			prefs.SetString("soundFilePath", path)
			soundPathLabel.SetText(path)
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp3", ".wav", ".ogg"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})

	resetSoundBtn := widget.NewButton("Reset", func() {
		prefs.RemoveValue("soundFilePath")
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
