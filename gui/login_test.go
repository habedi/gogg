package gui

import (
	"errors"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

type fakeLoginer struct {
	calls                int
	loginURL, user, pass string
	headless             bool
	err                  error

	codeCalls int
	code      string
	codeErr   error
}

func (f *fakeLoginer) Login(loginURL, username, password string, headless bool) error {
	f.calls++
	f.loginURL, f.user, f.pass, f.headless = loginURL, username, password, headless
	return f.err
}

func (f *fakeLoginer) LoginWithCode(codeOrURL string) error {
	f.codeCalls++
	f.code = codeOrURL
	return f.codeErr
}

// Starting a browser for empty credentials wastes seconds and ends in a
// confusing failure, so they are checked first.
func TestLoginToGOG_RequiresCredentials(t *testing.T) {
	loginer := &fakeLoginer{}

	require.Error(t, loginToGOG(loginer, "", "secret"))
	require.Error(t, loginToGOG(loginer, "   ", "secret"))
	require.Error(t, loginToGOG(loginer, "user", ""))
	require.Zero(t, loginer.calls, "no browser may be started without credentials")
}

func TestLoginToGOG_DrivesTheGogLoginFlow(t *testing.T) {
	loginer := &fakeLoginer{}

	require.NoError(t, loginToGOG(loginer, "user", "secret"))

	require.Equal(t, 1, loginer.calls)
	require.Equal(t, client.GOGLoginURL, loginer.loginURL)
	require.Equal(t, "user", loginer.user)
	require.Equal(t, "secret", loginer.pass)
	require.True(t, loginer.headless,
		"the headless attempt comes first; the client falls back to a visible window")
}

func TestLoginToGOG_ReportsFailure(t *testing.T) {
	loginer := &fakeLoginer{err: errors.New("invalid credentials")}

	require.ErrorContains(t, loginToGOG(loginer, "user", "secret"), "invalid credentials")
}

func collectButtons(o fyne.CanvasObject, out *[]*widget.Button) {
	switch v := o.(type) {
	case *widget.Button:
		*out = append(*out, v)
	case *fyne.Container:
		for _, c := range v.Objects {
			collectButtons(c, out)
		}
	case *widget.Card:
		if v.Content != nil {
			collectButtons(v.Content, out)
		}
	}
}

func buttonWithLabel(o fyne.CanvasObject, label string) *widget.Button {
	var buttons []*widget.Button
	collectButtons(o, &buttons)
	for _, b := range buttons {
		if b.Text == label {
			return b
		}
	}
	return nil
}

// Signing out of a terminal is not something a GUI user should have to do, so
// the library offers the login itself.
func TestLibraryTabUI_OffersLoginWhenSignedOut(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	defer win.Close()

	db.Path = filepath.Join(t.TempDir(), "games.db")
	require.NoError(t, db.InitDB())
	t.Cleanup(func() { _ = db.CloseDB() })

	requested := 0
	lt := LibraryTabUI(win, nil, &DownloadManager{Tasks: binding.NewUntypedList()},
		func() { requested++ })

	btn := buttonWithLabel(lt.content, "Log In to GOG")
	require.NotNil(t, btn, "the signed-out library must offer a way to log in")

	btn.OnTapped()
	require.Equal(t, 1, requested)
}

// The code flow is the one that needs no browser gogg can drive: the user logs
// in wherever they like and pastes the address back.
func TestLoginWithPastedCode_RequiresSomethingToWorkWith(t *testing.T) {
	loginer := &fakeLoginer{}

	require.Error(t, loginWithPastedCode(loginer, ""))
	require.Error(t, loginWithPastedCode(loginer, "   "))
	require.Zero(t, loginer.codeCalls)
}

func TestLoginWithPastedCode_PassesThePastedAddressThrough(t *testing.T) {
	loginer := &fakeLoginer{}
	const pasted = "https://embed.gog.com/on_login_success?origin=client&code=abc123"

	require.NoError(t, loginWithPastedCode(loginer, "  "+pasted+"  "))

	require.Equal(t, 1, loginer.codeCalls)
	require.Equal(t, pasted, loginer.code, "the address is trimmed but otherwise untouched")
	require.Zero(t, loginer.calls, "no browser may be driven for the code flow")
}

func TestLoginWithPastedCode_ReportsFailure(t *testing.T) {
	loginer := &fakeLoginer{codeErr: errors.New("invalid_grant")}

	require.ErrorContains(t, loginWithPastedCode(loginer, "abc123"), "invalid_grant")
}
