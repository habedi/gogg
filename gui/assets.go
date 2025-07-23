package gui

import (
	_ "embed"
	"fyne.io/fyne/v2"
)

//go:embed assets/game-card-svgrepo-com.svg
var logoSVG []byte

// AppLogo is the resource for the embedded logo.svg file.
var AppLogo = fyne.NewStaticResource("game-card-svgrepo-com.svg", logoSVG)
