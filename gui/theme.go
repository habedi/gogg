package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

var colorGoggBlue = &color.NRGBA{R: 0x0, G: 0x78, B: 0xD4, A: 0xff}

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
		return colorGoggBlue
	case theme.ColorNameFocus:
		return colorGoggBlue
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
