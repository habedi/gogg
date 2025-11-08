package gui

import (
	"fmt"
	"strings"

	"github.com/habedi/gogg/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func SettingsTabUI(win fyne.Window) fyne.CanvasObject {
	prefs := fyne.CurrentApp().Preferences()
	a := fyne.CurrentApp()

	// --- Theme Settings ---
	themeRadio := widget.NewRadioGroup([]string{"System Default", "Light", "Dark"}, func(selected string) {
		prefs.SetString("theme", selected)
		a.Settings().SetTheme(CreateThemeFromPreferences())
	})
	themeRadio.SetSelected(prefs.StringWithFallback("theme", "System Default"))

	themeBox := container.NewVBox(widget.NewLabel("UI Theme"), themeRadio)

	// --- Font Settings ---
	fontOptions := []string{
		"System Default",
		"JetBrains Mono",
		"JetBrains Mono Bold",
	}
	fontSelect := widget.NewSelect(fontOptions, func(selected string) {
		prefs.SetString("fontName", selected)
		a.Settings().SetTheme(CreateThemeFromPreferences())
	})
	fontSelect.SetSelected(prefs.StringWithFallback("fontName", "System Default"))

	fontSizeSelect := widget.NewSelect([]string{"Small", "Normal", "Large", "Extra Large"}, func(s string) {
		prefs.SetString("fontSize", s)
		a.Settings().SetTheme(CreateThemeFromPreferences())
	})
	fontSizeSelect.SetSelected(prefs.StringWithFallback("fontSize", "Normal"))

	fontBox := container.NewVBox(
		widget.NewLabel("Font Family"), fontSelect,
		widget.NewLabel("Font Size"), fontSizeSelect,
	)

	// --- Sound Settings ---
	soundCheck := widget.NewCheck("Play sound on download completion", func(checked bool) {
		prefs.SetBool("soundEnabled", checked)
	})
	soundCheck.SetChecked(prefs.BoolWithFallback("soundEnabled", true))

	soundPathLabel := widget.NewLabel("")
	soundStatusLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})

	validateSoundPath := func(path string) {
		if path == "" {
			soundPathLabel.SetText("Default sound file")
			soundStatusLabel.SetText("")
			soundStatusLabel.Hide()
			return
		}

		soundPathLabel.SetText(path)
		if err := validateAudioFile(path); err != nil {
			soundStatusLabel.SetText(fmt.Sprintf("⚠ %s. Using default.", err.Error()))
			soundStatusLabel.Show()
		} else {
			soundStatusLabel.SetText("✓ Valid audio file")
			soundStatusLabel.Show()
		}
	}
	validateSoundPath(prefs.String("soundFilePath"))

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

			if err := validateAudioFile(path); err != nil {
				errDialog := dialog.NewError(fmt.Errorf("invalid audio file: %w\n\nSupported formats: .mp3, .wav, .ogg\nMax size: 50MB", err), win)
				errDialog.SetDismissText("OK")
				errDialog.Show()
				return
			}

			prefs.SetString("soundFilePath", path)
			validateSoundPath(path)
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".mp3", ".wav", ".ogg"}))
		fd.Resize(fyne.NewSize(800, 600))
		fd.Show()
	})

	resetSoundBtn := widget.NewButton("Reset", func() {
		prefs.RemoveValue("soundFilePath")
		validateSoundPath("")
	})

	testSoundBtn := widget.NewButton("Test", func() {
		path := prefs.String("soundFilePath")
		if path != "" {
			if err := validateAudioFile(path); err != nil {
				dialog.ShowError(fmt.Errorf("can't play sound: %w", err), win)
				return
			}
		}
		go PlayNotificationSound()
	})

	soundConfigBox := container.NewVBox(
		widget.NewLabel("Current sound file:"),
		soundPathLabel,
		soundStatusLabel,
		widget.NewLabelWithStyle("Tip: Use short audio clips (2-5 seconds) for best results", fyne.TextAlignLeading, fyne.TextStyle{Italic: true}),
		container.NewHBox(selectSoundBtn, resetSoundBtn, testSoundBtn),
	)

	// --- Download Limits ---
	maxConcSelect := widget.NewSelect([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}, func(s string) {
		prefs.SetString("download.maxConcurrent", s)
	})
	maxConcSelect.SetSelected(fmt.Sprintf("%d", prefs.IntWithFallback("download.maxConcurrent", 2)))

	speedEntry := widget.NewEntry()
	speedEntry.SetPlaceHolder("Speed limit KB/s (0=unlimited)")
	if v := prefs.IntWithFallback("download.maxSpeedKBps", 0); v > 0 {
		speedEntry.SetText(fmt.Sprintf("%d", v))
	}
	speedEntry.OnChanged = func(s string) {
		if s == "" {
			prefs.SetInt("download.maxSpeedKBps", 0)
			client.SetGlobalDownloadRateLimit(0)
			return
		}
		var val int
		_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &val)
		if err != nil {
			return
		}
		prefs.SetInt("download.maxSpeedKBps", val)
		if val <= 0 {
			client.SetGlobalDownloadRateLimit(0)
		} else {
			client.SetGlobalDownloadRateLimit(int64(val) * 1024)
		}
	}
	limitsBox := container.NewVBox(widget.NewLabel("Download Limits"), widget.NewForm(
		widget.NewFormItem("Max Concurrent", maxConcSelect),
		widget.NewFormItem("Speed Limit", speedEntry),
	))

	// --- Layout ---
	mainCard := widget.NewCard("Settings", "", container.NewVBox(
		themeBox,
		widget.NewSeparator(),
		fontBox,
		widget.NewSeparator(),
		soundCheck,
		soundConfigBox,
		widget.NewSeparator(),
		limitsBox,
	))

	return container.NewCenter(mainCard)
}
