// gui/about.go
package gui

import (
	"fmt"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func ShowAboutUI(version string) fyne.CanvasObject {
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	goVersion := runtime.Version()
	lbl := widget.NewLabel(fmt.Sprintf(
		"GOGG: A multiplatform game file downloader for GOG\nVersion: %s, Platform: %s, Go Version: %s",
		version, platform, goVersion,
	))
	lbl.Alignment = fyne.TextAlignCenter
	lbl.TextStyle = fyne.TextStyle{Bold: true}

	copyright := widget.NewLabel("© 2025 Hassan Abedi")
	return container.NewVBox(
		container.NewCenter(lbl),
		container.NewCenter(copyright),
	)
}
