package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/require"
)

// The hash column had a width of its own whatever the window was, so a narrow
// window left the file path a sliver, and then a negative width that pushed the
// hash off the left edge.
func TestHashRow_SharesTheWidthWithTheFilePath(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	row := newHashRow()
	row.SetTexts("a/long/relative/path/to/an/installer.bin", "d41d8cd98f00b204e9800998ecf8427e")

	for _, width := range []float32{1200, 900, 640, 480} {
		row.Resize(fyne.NewSize(width, row.MinSize().Height))

		require.GreaterOrEqual(t, row.hash.Position().X, float32(0),
			"at %.0f points wide the hash column ran off the left edge", width)
		require.LessOrEqual(t, row.hash.Position().X+row.hash.Size().Width, width,
			"at %.0f points wide the hash column ran off the right edge", width)
		require.GreaterOrEqual(t, row.file.Size().Width, width/3,
			"at %.0f points wide the file path was squeezed out", width)
	}
}

// Every row works out its columns on its own, so the header has to land on the
// same split as the results under it.
func TestHashRow_HeaderLinesUpWithTheResults(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	header := newHashRow()
	header.SetTexts("File Path", "Hash (md5)")
	result := newHashRow()
	result.SetTexts("a/long/relative/path/to/an/installer.bin", "d41d8cd98f00b204e9800998ecf8427e")

	for _, width := range []float32{1200, 640} {
		header.Resize(fyne.NewSize(width, header.MinSize().Height))
		result.Resize(fyne.NewSize(width, result.MinSize().Height))

		require.Equal(t, header.hash.Position().X, result.hash.Position().X,
			"the columns have to line up at %.0f points wide", width)
	}
}

// A directory with nothing to hash has to say so. The progress bar appeared,
// filled nothing, and disappeared again with no results and no explanation.
func TestGenerateHashFiles_SaysWhenThereIsNothingToHash(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		err := generateHashFilesUI(t.TempDir(), "md5", true, 2,
			binding.NewUntypedList(), widget.NewProgressBar())

		require.Error(t, err)
		require.Contains(t, err.Error(), "no files")
	})
}

// A directory with files in it hashes them and says nothing.
func TestGenerateHashFiles_HashesWhatIsThere(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()

	offMain(t, func() {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "installer.bin"), []byte("hello"), 0o644))
		results := binding.NewUntypedList()

		require.NoError(t, generateHashFilesUI(dir, "md5", true, 2, results, widget.NewProgressBar()))

		items, err := results.Get()
		require.NoError(t, err)
		require.Len(t, items, 1)
	})
}

// The tab already names the section, so the pane under it does not repeat it.
func TestFileTabUI_DoesNotRepeatTheSectionName(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	require.NotContains(t, labelTexts(FileTabUI(win)), sectionFileHashes,
		"the tab says what this is; the pane does not have to say it again")
}

// The hash utility opens from the library's More menu as a dialog, since it
// is no longer a tab of its own.
func TestShowFileHashes_OpensAsADialog(t *testing.T) {
	app := test.NewApp()
	defer app.Quit()
	win := test.NewWindow(nil)
	win.Resize(fyne.NewSize(defaultWindowWidth, defaultWindowHeight))
	t.Cleanup(win.Close)

	showFileHashes(win)

	overlay := win.Canvas().Overlays().Top()
	require.NotNil(t, overlay, "the utility opens over the window")
	require.NotNil(t, buttonWithLabel(overlay, "Generate File Hashes"),
		"and it is the hash utility that opened")
}
