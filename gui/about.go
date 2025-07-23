package gui

import (
	"fmt"
	"net/url"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

var goggRepo = "https://github.com/habedi/gogg"

func ShowAboutUI(version string) fyne.CanvasObject {
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	goVersion := runtime.Version()

	versionLbl := widget.NewLabel(fmt.Sprintf("Version: %s", version))
	goLbl := widget.NewLabel(fmt.Sprintf("Go version: %s", goVersion))
	platformLbl := widget.NewLabel(fmt.Sprintf("Platform: %s", platform))

	repoURL, _ := url.Parse(goggRepo)
	repoLink := widget.NewHyperlink("Project's GitHub Repository", repoURL)

	info := container.NewVBox(
		versionLbl,
		goLbl,
		platformLbl,
		repoLink,
	)

	author := widget.NewLabel("© 2025 Hassan Abedi")

	titleLbl := widget.NewLabelWithStyle("Gogg", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	subtitleLbl := widget.NewLabelWithStyle("A game file downloader for GOG",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	card := widget.NewCard(
		"",
		"",
		container.NewVBox(
			container.NewVBox(
				titleLbl,
				subtitleLbl,
			),
			info,
			author,
		),
	)

	return container.NewCenter(card)
}
