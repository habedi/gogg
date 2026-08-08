package client

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The markup GOG writes descriptions in survives as what a rich text widget
// can set: emphasis, headings, and lists.
func TestMarkdownText(t *testing.T) {
	md := markdownText(`<h4>About</h4><p><b>Bold</b> and <i>slanted</i> text.</p><ul><li>one</li><li>two</li></ul>`)

	require.Equal(t, "## About\n\n**Bold** and *slanted* text.\n\n- one\n- two", md)
}

func TestMarkdownText_PlainStaysPlain(t *testing.T) {
	require.Equal(t, "Just words.", markdownText("Just words."))
	require.Empty(t, markdownText(""))
}

// Entities decode after the tags go, the same as in the plain rendering.
func TestMarkdownText_DecodesEntities(t *testing.T) {
	require.Equal(t, "Gods & monsters", markdownText("Gods &amp; monsters"))
}
