package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Gogg's accent is purple, one shade per variant so text stays readable on
// it: a deep purple carries white text in the light theme, and a light
// purple carries dark text in the dark theme. The pairs come from the
// Material palette, where their contrast is a settled matter, and the same
// ratios are pinned by TestThemeContrast.
var (
	colorPurpleDeep    = &color.NRGBA{R: 0x67, G: 0x50, B: 0xA4, A: 0xff}
	colorPurpleLight   = &color.NRGBA{R: 0xD0, G: 0xBC, B: 0xFF, A: 0xff}
	colorOnPurpleDeep  = &color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xff}
	colorOnPurpleLight = &color.NRGBA{R: 0x38, G: 0x1E, B: 0x72, A: 0xff}
)

// accentFor is the purple for a variant, and onAccentFor the text that sits
// on top of it.
func accentFor(v fyne.ThemeVariant) *color.NRGBA {
	if v == theme.VariantDark {
		return colorPurpleLight
	}
	return colorPurpleDeep
}

func onAccentFor(v fyne.ThemeVariant) *color.NRGBA {
	if v == theme.VariantDark {
		return colorOnPurpleLight
	}
	return colorOnPurpleDeep
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

	// Custom color overrides
	switch name {
	case theme.ColorNamePrimary:
		return accentFor(finalVariant)
	case theme.ColorNameForegroundOnPrimary:
		return onAccentFor(finalVariant)
	case theme.ColorNameHyperlink:
		return accentFor(finalVariant)
	case theme.ColorNameFocus:
		return withAlpha(accentFor(finalVariant), 0x99)
	case theme.ColorNameSelection:
		// The tint behind selected rows is the light purple in both
		// variants: the deep one over a light background costs the row's
		// text its contrast, as TestThemeContrast will attest.
		if finalVariant == theme.VariantDark {
			return withAlpha(colorPurpleLight, 0x3d)
		}
		return withAlpha(colorPurpleLight, 0x55)
	case theme.ColorNameForeground:
		// Fyne's light theme sets text in a middle gray that reads poorly,
		// and worse over any tint. Near-black holds its contrast anywhere.
		if finalVariant == theme.VariantLight {
			return &color.NRGBA{R: 0x1C, G: 0x1B, B: 0x1F, A: 0xff}
		}
	case theme.ColorNameSeparator:
		if finalVariant == theme.VariantDark {
			return &color.NRGBA{R: 0x4A, B: 0x4A, G: 0x4A, A: 0xff} // Darker gray for dark mode
		}
		return &color.NRGBA{R: 0xD0, B: 0xD0, G: 0xD0, A: 0xff} // Lighter gray for light mode
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

	customTheme := &GoggTheme{Theme: theme.DefaultTheme()}

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
