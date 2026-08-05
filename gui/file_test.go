package gui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
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
