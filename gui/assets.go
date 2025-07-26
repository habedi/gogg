package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/game-card-svgrepo-com.svg
var logoSVG []byte

// AppLogo is the resource for the embedded logo.svg file.
var AppLogo = fyne.NewStaticResource("game-card-svgrepo-com.svg", logoSVG)

// --- Embedded JetBrains Mono Fonts ---

//go:embed assets/JetBrainsMono-2.304/fonts/ttf/JetBrainsMono-Regular.ttf
var jetbrainsMonoRegularFont []byte

//go:embed assets/JetBrainsMono-2.304/fonts/ttf/JetBrainsMono-Bold.ttf
var jetbrainsMonoBoldFont []byte

var (
	JetBrainsMonoRegular = &fyne.StaticResource{StaticName: "JetBrainsMono-Regular.ttf", StaticContent: jetbrainsMonoRegularFont}
	JetBrainsMonoBold    = &fyne.StaticResource{StaticName: "JetBrainsMono-Bold.ttf", StaticContent: jetbrainsMonoBoldFont}
)
