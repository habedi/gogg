package gui

import (
	"net/url"

	"fyne.io/fyne/v2"
)

// runOnMain schedules fn to run on the main Fyne thread
func runOnMain(fn func()) {
	fyne.Do(fn)
}

// parseURL safely parses a URL string
func parseURL(urlStr string) *url.URL {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil
	}
	return u
}
