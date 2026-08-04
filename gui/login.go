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
	// Login drives a browser through the GOG login form.
	Login(loginURL, username, password string, headless bool) error
	// LoginWithCode exchanges an authorization code, or the address containing
	// one, for tokens.
	LoginWithCode(codeOrURL string) error
}

// loginToGOG signs in with credentials by driving a browser. The headless
// attempt comes first; the client falls back to a visible browser window by
// itself when GOG asks for something a headless browser cannot answer, such as
// a captcha.
func loginToGOG(loginer GogLoginer, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return errors.New("username and password are required")
	}
	return loginer.Login(client.GOGLoginURL, username, password, true)
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

// ShowLoginDialog walks the user through logging in with their own browser and
// pasting back the address they land on. The credential flow, which needs a
// browser gogg can drive, is offered as an alternative.
func ShowLoginDialog(win fyne.Window, loginer GogLoginer, onSuccess func()) {
	var dlg *dialog.CustomDialog

	address := widget.NewEntry()
	address.SetPlaceHolder("https://embed.gog.com/on_login_success?...&code=...")

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

	credentialsBtn := widget.NewButton("Use username and password instead", func() {
		dlg.Hide()
		showCredentialLogin(win, loginer, onSuccess)
	})
	credentialsBtn.Importance = widget.LowImportance

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

		widget.NewLabelWithStyle("3. Paste that page's address here",
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		address,
		widget.NewSeparator(),

		container.NewBorder(nil, nil, credentialsBtn, submitBtn),
	)

	dlg = dialog.NewCustom("Log In to GOG", "Cancel", content, win)
	dlg.Resize(fyne.NewSize(620, 560))
	dlg.Show()
}

// showCredentialLogin signs in by driving a browser through the login form.
// The credentials are used for this one login and never stored; only the tokens
// GOG returns are saved.
func showCredentialLogin(win fyne.Window, loginer GogLoginer, onSuccess func()) {
	username := widget.NewEntry()
	username.SetPlaceHolder("GOG username or email")
	password := widget.NewPasswordEntry()
	password.SetPlaceHolder("GOG password")

	items := []*widget.FormItem{
		widget.NewFormItem("Username", username),
		widget.NewFormItem("Password", password),
	}

	form := dialog.NewForm("Log In with Credentials", "Log In", "Cancel", items, func(confirmed bool) {
		if !confirmed {
			return
		}
		user, pass := username.Text, password.Text
		runLoginAttempt(win, "A browser window may open so you can finish signing in.",
			func() error { return loginToGOG(loginer, user, pass) }, onSuccess)
	}, win)
	form.Resize(fyne.NewSize(420, 220))
	form.Show()
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
