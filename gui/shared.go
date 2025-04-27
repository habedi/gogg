package gui

import (
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// runOnMain schedules fn to run on the main Fyne thread
func runOnMain(fn func()) {
	fyne.Do(fn)
}

// runOnMain schedules fn to run on the main Fyne thread
func runOnMainV2(fn func()) {
	fyne.DoAndWait(fn)
}

// appendLog adds a line to the multi‐line entry, trimming to ~8000 runes.
func appendLog(logOutput *widget.Entry, msg string) {
	const maxRunes = 8000
	runOnMain(func() {
		cur := logOutput.Text
		var b strings.Builder
		b.Grow(len(cur) + len(msg) + 1)
		b.WriteString(cur)
		b.WriteString(msg)
		b.WriteString("\n")
		nw := b.String()
		if utf8.RuneCountInString(nw) > maxRunes {
			r := []rune(nw)
			start := len(r) - maxRunes
			if start < 0 {
				start = 0
			}
			t := string(r[start:])
			if idx := strings.Index(t, "\n"); idx >= 0 && idx+1 < len(t) {
				nw = t[idx+1:]
			} else {
				nw = t
			}
		}
		logOutput.SetText(nw)
	})
}
