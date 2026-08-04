package gui

import (
	"errors"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/db"
	"github.com/stretchr/testify/require"
)

type fakeLoginer struct {
	codeCalls int
	code      string
	codeErr   error
}

func (f *fakeLoginer) LoginWithCode(codeOrURL string) error {
	f.codeCalls++
	f.code = codeOrURL
	return f.codeErr
}

// The GUI logs in by having the user sign in with their own browser and paste
// the address back, so gogg never handles the password itself.
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
}

func TestLoginWithPastedCode_ReportsFailure(t *testing.T) {
	loginer := &fakeLoginer{codeErr: errors.New("invalid_grant")}

	require.ErrorContains(t, loginWithPastedCode(loginer, "abc123"), "invalid_grant")
}

// The address is long and always arrives through the clipboard, so pasting it
// is one button rather than a manual selection.
func TestFillFromClipboard(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	const pasted = "https://embed.gog.com/on_login_success?origin=client&code=abc123"
	app.Clipboard().SetContent("  " + pasted + "  ")

	entry := widget.NewEntry()
	fillFromClipboard(entry)

	require.Equal(t, pasted, entry.Text)
}

func TestFillFromClipboard_EmptyClipboardKeepsWhatIsThere(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	app.Clipboard().SetContent("   ")

	entry := widget.NewEntry()
	entry.SetText("already typed")
	fillFromClipboard(entry)

	require.Equal(t, "already typed", entry.Text)
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

// Signing in from a terminal is not something a GUI user should have to do, so
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
