package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// GoggTheme defines a custom theme that supports color variants, custom fonts, and sizes.
type GoggTheme struct {
	fyne.Theme
	variant *fyne.ThemeVariant // Pointer to allow for nil (system default)

	regular, bold, italic, boldItalic, monospace fyne.Resource
	textSize                                     float32
}

// Color overrides the default to use our forced variant, or passes through if we want the system default.
func (t *GoggTheme) Color(name fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	if t.variant != nil {
		return t.Theme.Color(name, *t.variant)
	}
	return t.Theme.Color(name, v)
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
	// If "System Default", customTheme.variant remains nil, which is the correct behavior.

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
