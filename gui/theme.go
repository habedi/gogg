package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Every accent gogg offers is a pair of shades, one per theme, so text
// stays readable on it: the deep shade carries white text in the light
// theme, and the light shade carries dark text in the dark theme. The pairs
// come from the Material palette, where their contrast is a settled matter,
// and TestThemeContrast pins every hue in both themes.
type accentPair struct {
	deep, onDeep, light, onLight *color.NRGBA
}

func rgb(r, g, b uint8) *color.NRGBA { return &color.NRGBA{R: r, G: g, B: b, A: 0xff} }

var colorWhite = rgb(0xFF, 0xFF, 0xFF)

// accentNames is the order the settings offer the hues in; the first is the
// default.
var accentNames = []string{"Purple", "Blue", "Teal", "Green", "Amber", "Pink"}

// prefAccentColor stores which hue the user picked.
const prefAccentColor = "accentColor"

var accentPairs = map[string]accentPair{
	"Purple": {deep: rgb(0x67, 0x50, 0xA4), onDeep: colorWhite, light: rgb(0xD0, 0xBC, 0xFF), onLight: rgb(0x38, 0x1E, 0x72)},
	"Blue":   {deep: rgb(0x00, 0x61, 0xA4), onDeep: colorWhite, light: rgb(0x9E, 0xCA, 0xFF), onLight: rgb(0x00, 0x32, 0x58)},
	"Teal":   {deep: rgb(0x00, 0x69, 0x6E), onDeep: colorWhite, light: rgb(0x4F, 0xD8, 0xEB), onLight: rgb(0x00, 0x36, 0x3D)},
	"Green":  {deep: rgb(0x00, 0x6E, 0x1C), onDeep: colorWhite, light: rgb(0x78, 0xDC, 0x77), onLight: rgb(0x00, 0x39, 0x0A)},
	"Amber":  {deep: rgb(0x8B, 0x50, 0x00), onDeep: colorWhite, light: rgb(0xFF, 0xB8, 0x70), onLight: rgb(0x4A, 0x28, 0x00)},
	"Pink":   {deep: rgb(0x98, 0x40, 0x61), onDeep: colorWhite, light: rgb(0xFF, 0xB1, 0xC8), onLight: rgb(0x5E, 0x11, 0x33)},
}

// accentByName is the pair for a stored preference, falling back to the
// default for names from a version that spelled them differently.
func accentByName(name string) accentPair {
	if pair, ok := accentPairs[name]; ok {
		return pair
	}
	return accentPairs[accentNames[0]]
}

// accent is this theme's pair, defaulting when the theme was built without
// one, as tests do.
func (t *GoggTheme) accentColors() accentPair {
	if t.accent.deep == nil {
		return accentByName("")
	}
	return t.accent
}

// withAlpha is the accent thinned out, for fills that sit behind text: a
// selection tint or a focus ring must color a row without costing the words
// on it their contrast.
func withAlpha(c *color.NRGBA, a uint8) *color.NRGBA {
	return &color.NRGBA{R: c.R, G: c.G, B: c.B, A: a}
}

// GoggTheme defines a custom theme that supports color variants, custom fonts, and sizes.
type GoggTheme struct {
	fyne.Theme
	variant *fyne.ThemeVariant // Pointer to allow for nil (system default)
	accent  accentPair         // Zero means the default hue.

	regular, bold, italic, boldItalic, monospace fyne.Resource
	textSize                                     float32
}

// Color overrides the default to use our forced variant and custom colors.
func (t *GoggTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	// Determine the final variant to use (forced or system)
	finalVariant := v
	if t.variant != nil {
		finalVariant = *t.variant
	}

	accent := t.accentColors()
	dark := finalVariant == theme.VariantDark

	// Custom color overrides
	switch name {
	case theme.ColorNamePrimary, theme.ColorNameHyperlink:
		if dark {
			return accent.light
		}
		return accent.deep
	case theme.ColorNameForegroundOnPrimary:
		if dark {
			return accent.onLight
		}
		return accent.onDeep
	case theme.ColorNameFocus:
		if dark {
			return withAlpha(accent.light, 0x99)
		}
		return withAlpha(accent.deep, 0x99)
	case theme.ColorNameSelection:
		// The tint behind selected rows is the light shade in both themes:
		// the deep one over a light background costs the row's text its
		// contrast, as TestThemeContrast will attest.
		if dark {
			return withAlpha(accent.light, 0x3d)
		}
		return withAlpha(accent.light, 0x55)
	case theme.ColorNameForeground:
		// Fyne's light theme sets text in a middle gray that reads poorly,
		// and worse over any tint. Near-black holds its contrast anywhere.
		if !dark {
			return rgb(0x1C, 0x1B, 0x1F)
		}
	case theme.ColorNameBackground:
		// A moderately dark page, not a near-black one. Cards separate
		// themselves with a drop shadow, and a black shadow shows on a page
		// this dark but vanishes on a near-black one; the earlier, darker
		// page was why panels lost their edges. Interactive surfaces sit a
		// clear step above this, so they still read as raised.
		if dark {
			return rgb(0x1B, 0x1C, 0x20)
		}
	case theme.ColorNameInputBackground, theme.ColorNameButton:
		// Entries, selects, and the plain buttons share one tone, a clear
		// step up from the page, so they read as raised fields rather than
		// patches of the same dark. The gap is held open by TestThemeSurfaces.
		if dark {
			return rgb(0x33, 0x34, 0x3B)
		}
	case theme.ColorNameSeparator:
		// Fyne's own separators are black on near-black in the dark theme
		// and barely-there gray in the light one; these stay quiet but can
		// be seen, and TestThemeBorders holds them to it.
		if dark {
			return rgb(0x5A, 0x5A, 0x62)
		}
		return rgb(0xBF, 0xBF, 0xC6)
	case theme.ColorNameInputBorder:
		// The outline of a field carries its definition, since a fill a step
		// off the page is subtle on its own. Fyne's dark default sat at
		// 1.5:1 against the background; this reads clearly against both the
		// page and the field's own fill, which TestThemeBorders checks.
		if dark {
			return rgb(0x8A, 0x8A, 0x92)
		}
		return rgb(0x76, 0x76, 0x80)
	}

	// Fallback to the default theme for all other colors, using the correct variant.
	return t.Theme.Color(name, finalVariant)
}

// Icon demonstrates how to override a default Fyne icon.
func (t *GoggTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	// Example: Override the settings icon.
	// To make this work, you would need to:
	// 1. Add a "custom_settings.svg" file to your assets.
	// 2. Embed it in `gui/assets.go` like the other assets.
	// 3. Uncomment the following lines.
	//
	// if name == theme.IconNameSettings {
	// 	 return YourCustomSettingsIconResource
	// }

	// Fallback to the default theme for all other icons
	return t.Theme.Icon(name)
}

func (t *GoggTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.regular == nil {
		return t.Theme.Font(style) // Fallback to default theme's font
	}

	if style.Monospace && t.monospace != nil {
		return t.monospace
	}
	if style.Bold && style.Italic && t.boldItalic != nil {
		return t.boldItalic
	}
	if style.Bold && t.bold != nil {
		return t.bold
	}
	if style.Italic && t.italic != nil {
		return t.italic
	}
	return t.regular
}

func (t *GoggTheme) Size(name fyne.ThemeSizeName) float32 {
	if t.textSize > 0 {
		if name == theme.SizeNameText {
			return t.textSize
		}
	}
	// Rounder corners than the Fyne defaults, so inputs, buttons, and
	// selections read as soft cards rather than boxes.
	switch name {
	case theme.SizeNameInputRadius:
		return 8
	case theme.SizeNameSelectionRadius:
		return 6
	}
	return t.Theme.Size(name)
}

// CreateThemeFromPreferences reads all UI preferences and constructs the appropriate theme.
func CreateThemeFromPreferences() fyne.Theme {
	prefs := fyne.CurrentApp().Preferences()
	variantName := prefs.StringWithFallback("theme", "System Default")
	fontName := prefs.StringWithFallback("fontName", "System Default")
	sizeName := prefs.StringWithFallback("fontSize", "Normal")

	customTheme := &GoggTheme{
		Theme:  theme.DefaultTheme(),
		accent: accentByName(prefs.StringWithFallback(prefAccentColor, accentNames[0])),
	}

	// Set color variant
	switch variantName {
	case "Light":
		lightVariant := theme.VariantLight
		customTheme.variant = &lightVariant
	case "Dark":
		darkVariant := theme.VariantDark
		customTheme.variant = &darkVariant
	}

	// Set font family and weight
	switch fontName {
	case "JetBrains Mono":
		customTheme.regular = JetBrainsMonoRegular
		customTheme.bold = JetBrainsMonoBold
		customTheme.monospace = JetBrainsMonoRegular
	case "JetBrains Mono Bold":
		customTheme.regular = JetBrainsMonoBold
		customTheme.bold = JetBrainsMonoBold // It's already bold, so bold style is the same
		customTheme.monospace = JetBrainsMonoBold
	}

	// Set size
	switch sizeName {
	case "Small":
		customTheme.textSize = 12
	case "Normal":
		customTheme.textSize = 14
	case "Large":
		customTheme.textSize = 16
	case "Extra Large":
		customTheme.textSize = 18
	}

	return customTheme
}
