package gui

import (
	"errors"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
)

// GogLoginer signs in to GOG and stores the resulting tokens.
// *client.GogClient implements it.
type GogLoginer interface {
	// LoginWithCode exchanges an authorization code, or the address containing
	// one, for tokens.
	LoginWithCode(codeOrURL string) error
}

// loginWithPastedCode signs in with the address GOG redirected the user's own
// browser to. Nothing here drives a browser, so it works wherever the user can
// open a web page, and the password never passes through gogg.
func loginWithPastedCode(loginer GogLoginer, pasted string) error {
	pasted = strings.TrimSpace(pasted)
	if pasted == "" {
		return errors.New("paste the address of the page you landed on after logging in")
	}
	return loginer.LoginWithCode(pasted)
}

// fillFromClipboard puts the clipboard contents into entry, leaving whatever is
// already there alone when the clipboard holds nothing useful.
func fillFromClipboard(entry *widget.Entry) {
	if text := strings.TrimSpace(fyne.CurrentApp().Clipboard().Content()); text != "" {
		entry.SetText(text)
	}
}

// ShowLoginDialog walks the user through logging in with their own browser and
// pasting back the address they land on.
func ShowLoginDialog(win fyne.Window, loginer GogLoginer, onSuccess func()) {
	var dlg *dialog.CustomDialog

	address := widget.NewEntry()
	address.SetPlaceHolder("https://embed.gog.com/on_login_success?...&code=...")

	pasteBtn := widget.NewButtonWithIcon("Paste", theme.ContentPasteIcon(), func() {
		fillFromClipboard(address)
	})

	loginURL := NewCopyableLabel(client.GOGLoginURL)
	loginURL.Wrapping = fyne.TextWrapBreak

	openBtn := widget.NewButton("Open GOG Login Page", func() {
		parsed := parseURL(client.GOGLoginURL)
		if parsed == nil {
			return
		}
		if err := fyne.CurrentApp().OpenURL(parsed); err != nil {
			showErrorDialog(win, "Could not open your browser", err)
		}
	})

	submitBtn := widget.NewButtonWithIcon("Log In", theme.LoginIcon(), func() {
		pasted := address.Text
		dlg.Hide()
		runLoginAttempt(win, "Exchanging the authorization code for an access token.",
			func() error { return loginWithPastedCode(loginer, pasted) }, onSuccess)
	})
	submitBtn.Importance = widget.HighImportance

	content := container.NewVBox(
		widget.NewLabelWithStyle("1. Open the GOG login page in your browser",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		openBtn,
		widget.NewLabel("If nothing opens, tap this address to copy it:"),
		loginURL,
		widget.NewSeparator(),

		widget.NewLabelWithStyle("2. Log in to GOG there",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel("You will land on a page that may look blank. That is expected."),
		widget.NewSeparator(),

		widget.NewLabelWithStyle("3. Copy that page's address and paste it here",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewBorder(nil, nil, nil, pasteBtn, address),
		widget.NewSeparator(),

		container.NewBorder(nil, nil, nil, submitBtn),
	)

	dlg = dialog.NewCustom("Log In to GOG", "Cancel", content, win)
	dlg.Resize(fyne.NewSize(620, 540))
	dlg.Show()
}

// runLoginAttempt performs a login off the UI thread and reports the outcome.
func runLoginAttempt(win fyne.Window, waitingMessage string, attempt func() error, onSuccess func()) {
	progress := dialog.NewCustom("Logging In", "Hide", container.NewVBox(
		widget.NewProgressBarInfinite(),
		widget.NewLabel(waitingMessage),
	), win)
	progress.Show()

	go func() {
		err := attempt()

		runOnMain(func() {
			progress.Hide()
			if err != nil {
				showErrorDialog(win, "Login failed", err)
				return
			}
			dialog.ShowInformation("Logged In", "You are signed in to GOG.", win)
			if onSuccess != nil {
				onSuccess()
			}
		})
	}()
}
