package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/habedi/gogg/client"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// SettingsTabUI builds the settings. onSignOut is called once the user has
// signed out, so the rest of the app can go back to its signed-out self.
func SettingsTabUI(win fyne.Window, st stores, onSignOut func()) fyne.CanvasObject {
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

	notifyCheck := widget.NewCheck("Show a notification on download completion", func(checked bool) {
		prefs.SetBool(prefNotifications, checked)
	})
	notifyCheck.SetChecked(prefs.BoolWithFallback(prefNotifications, true))

	sweepCheck := widget.NewCheck("Fetch store details in the background", func(checked bool) {
		prefs.SetBool(prefMetadataSweep, checked)
	})
	sweepCheck.SetChecked(prefs.BoolWithFallback(prefMetadataSweep, true))

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
		fd.Resize(fileDialogSize)
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
		if v, err := strconv.Atoi(s); err == nil {
			prefs.SetInt(prefMaxConcurrent, v)
		}
	})
	maxConcSelect.SetSelected(fmt.Sprintf("%d", maxConcurrentDownloads(prefs)))

	speedEntry := widget.NewEntry()
	speedEntry.SetPlaceHolder("No limit")
	// A limit that is not a number is ignored, leaving the last one in force,
	// so the box has to show that what it says is not what is happening.
	speedEntry.Validator = func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		if value, err := strconv.Atoi(text); err != nil || value < 0 {
			return errors.New("a whole number of kilobytes a second, or nothing for no limit")
		}
		return nil
	}
	if v := prefs.IntWithFallback(prefMaxSpeedKBps, 0); v > 0 {
		speedEntry.SetText(fmt.Sprintf("%d", v))
	}
	// SetText above runs before OnChanged is assigned, so the stored limit has
	// to be put into force explicitly.
	applySavedSpeedLimit(prefs)
	speedEntry.OnChanged = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			prefs.SetInt(prefMaxSpeedKBps, 0)
			applySpeedLimit(0)
			return
		}
		val, err := strconv.Atoi(s)
		if err != nil || val < 0 {
			return
		}
		prefs.SetInt(prefMaxSpeedKBps, val)
		applySpeedLimit(val)
	}
	limitsBox := container.NewVBox(widget.NewLabel("Download Limits"), widget.NewForm(
		widget.NewFormItem("Max Concurrent", maxConcSelect),
		widget.NewFormItem("Speed Limit (KB/s)", speedEntry),
	))

	// --- Update Detection ---
	// These decide what counts as an update for every game, so they belong with
	// the settings rather than behind a button in the library toolbar. The
	// library is told, because what it worked out under the old rules is no
	// longer the answer.
	updateChecks := make([]fyne.CanvasObject, 0, 5)
	updateChecks = append(updateChecks, widget.NewLabel("Update Detection"))
	for _, option := range []struct {
		label    string
		pref     string
		fallback bool
	}{
		{"Include extras in update check", "downloadForm.includeExtrasUpdates", false},
		{"Include DLCs in update check", "downloadForm.includeDLCUpdates", false},
		{"Include patches", "downloadForm.includePatchUpdates", false},
		{"Scan folders when history missing", "downloadForm.scanDirsForDownloads", true},
	} {
		key := option.pref
		check := widget.NewCheck(option.label, func(checked bool) {
			prefs.SetBool(key, checked)
			SignalUpdateSettingsChanged()
		})
		check.SetChecked(prefs.BoolWithFallback(key, option.fallback))
		updateChecks = append(updateChecks, check)
	}
	updatesBox := container.NewVBox(updateChecks...)

	// --- Layout ---
	sections := []fyne.CanvasObject{
		themeBox,
		widget.NewSeparator(),
		fontBox,
		widget.NewSeparator(),
		soundCheck,
		notifyCheck,
		sweepCheck,
		soundConfigBox,
		widget.NewSeparator(),
		limitsBox,
		widget.NewSeparator(),
		updatesBox,
	}
	if account := accountBox(win, st, onSignOut); account != nil {
		sections = append(sections, widget.NewSeparator(), account)
	}
	mainCard := widget.NewCard("Settings", "", container.NewVBox(sections...))

	// The settings are taller than the window gogg opens at. Centred and fixed
	// in place, the download limits sat below the bottom edge with no way to
	// reach them, so the tab scrolls.
	return container.NewVScroll(container.NewCenter(mainCard))
}

const prefMaxSpeedKBps = "download.maxSpeedKBps"

// applySavedSpeedLimit puts the stored download speed limit into force.
func applySavedSpeedLimit(prefs fyne.Preferences) {
	applySpeedLimit(prefs.IntWithFallback(prefMaxSpeedKBps, 0))
}

// applySpeedLimit throttles downloads to kbps kilobytes per second; anything
// below one means unlimited.
func applySpeedLimit(kbps int) {
	if kbps <= 0 {
		client.SetGlobalDownloadRateLimit(0)
		return
	}
	client.SetGlobalDownloadRateLimit(int64(kbps) * 1024)
}

// accountBox offers the way out of an account. There is nothing to offer when
// nobody is signed in, so it is left out then.
func accountBox(win fyne.Window, st stores, onSignOut func()) fyne.CanvasObject {
	token, err := st.tokens.Get(context.Background())
	if err != nil || token == nil {
		return nil
	}

	signOut := widget.NewButtonWithIcon("Log Out", theme.LogoutIcon(), func() {
		dialog.ShowConfirm("Log Out",
			"Sign out of GOG? The catalogue gogg has already fetched stays where it is.",
			func(confirmed bool) {
				if !confirmed {
					return
				}
				if err := st.tokens.Delete(context.Background()); err != nil {
					showErrorDialog(win, "Could not sign out", err)
					return
				}
				if onSignOut != nil {
					onSignOut()
				}
			}, win)
	})

	return container.NewVBox(widget.NewLabel("Account"), container.NewHBox(signOut))
}
