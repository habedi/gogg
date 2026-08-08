package gui

import (
	"fmt"
	"net/url"
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var goggRepo = "https://github.com/habedi/gogg"

func ShowAboutUI(version string) fyne.CanvasObject {
	platform := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	goVersion := runtime.Version()

	logoImage := canvas.NewImageFromResource(AppLogo)
	logoImage.SetMinSize(fyne.NewSize(96, 96))
	logoImage.FillMode = canvas.ImageFillContain

	titleLbl := widget.NewLabelWithStyle(appName, fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true})
	titleLbl.SizeName = theme.SizeNameHeadingText
	subtitleLbl := widget.NewLabelWithStyle("A Game File Downloader for GOG",
		fyne.TextAlignCenter, fyne.TextStyle{Italic: true})

	// The facts sit in a form so their values line up under one another
	// rather than trailing off their labels at ragged starts. The version is
	// copyable, since it is the one people are asked to quote in bug reports.
	versionValue := NewCopyableLabel(version)
	info := widget.NewForm(
		widget.NewFormItem("Version", versionValue),
		widget.NewFormItem("Go", widget.NewLabel(goVersion)),
		widget.NewFormItem("Platform", widget.NewLabel(platform)),
	)

	// The links people actually reach for from an About page: the project,
	// where to report a problem, and the documentation.
	repoURL, _ := url.Parse(goggRepo)
	issuesURL, _ := url.Parse(goggRepo + "/issues")
	docsURL, _ := url.Parse(goggRepo + "/blob/main/docs/README.md")
	links := container.NewHBox(
		layout.NewSpacer(),
		widget.NewHyperlink("Home Page", repoURL),
		widget.NewLabel("·"),
		widget.NewHyperlink("Report an Issue", issuesURL),
		widget.NewLabel("·"),
		widget.NewHyperlink("Documentation", docsURL),
		layout.NewSpacer(),
	)

	// No year: one written into the binary is wrong the January after it
	// ships.
	footer := widget.NewLabelWithStyle("© Hassan Abedi · MIT License",
		fyne.TextAlignCenter, fyne.TextStyle{})

	card := widget.NewCard("", "", container.NewVBox(
		container.NewCenter(logoImage),
		titleLbl,
		subtitleLbl,
		widget.NewSeparator(),
		info,
		widget.NewSeparator(),
		links,
		footer,
	))

	// Held to a sensible width so the form does not stretch across a wide
	// window, and scrolls on a short one.
	sized := container.NewGridWrap(fyne.NewSize(420, card.MinSize().Height), card)
	return container.NewScroll(container.NewCenter(sized))
}
