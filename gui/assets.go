package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Material Design star shapes, carried here because Fyne's theme has no
// favorite icon. Themed, so they follow the theme's colors like the rest.
var (
	starFilledSVG  = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000000" d="M12 17.27 18.18 21l-1.64-7.03L22 9.24l-7.19-.61L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21z"/></svg>`)
	starOutlineSVG = []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000000" d="m22 9.24-7.19-.62L12 2 9.19 8.63 2 9.24l5.46 4.73L5.82 21 12 17.27 18.18 21l-1.63-7.03zM12 15.4l-3.76 2.27 1-4.28-3.32-2.88 4.38-.38L12 6.1l1.71 4.04 4.38.38-3.32 2.88 1 4.28z"/></svg>`)

	iconStarFilled  = theme.NewPrimaryThemedResource(fyne.NewStaticResource("star-filled.svg", starFilledSVG))
	iconStarOutline = theme.NewThemedResource(fyne.NewStaticResource("star-outline.svg", starOutlineSVG))
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
