package gui

import (
	"image/color"
	"math"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/stretchr/testify/require"
)

// luminance is the WCAG relative luminance of a color.
func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	channel := func(v uint32) float64 {
		s := float64(v) / 0xffff
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(r) + 0.7152*channel(g) + 0.0722*channel(b)
}

// contrast is the WCAG contrast ratio between two colors, 1 to 21.
func contrast(a, b color.Color) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// over composites a translucent fill onto an opaque background, which is what
// the eye sees behind selected rows.
func over(fill, background color.Color) color.Color {
	// RGBA returns premultiplied channels, so the fill's values already
	// carry its alpha; only the background is scaled here.
	fr, fg, fb, fa := fill.RGBA()
	br, bg, bb, _ := background.RGBA()
	alpha := float64(fa) / 0xffff
	blend := func(f, b uint32) uint8 {
		return uint8((float64(f)/0xffff + float64(b)/0xffff*(1-alpha)) * 0xff)
	}
	return color.NRGBA{R: blend(fr, br), G: blend(fg, bg), B: blend(fb, bb), A: 0xff}
}

// Text must stay readable in both themes and under every accent hue: on the
// plain background, on the accent, and on a selected row. The 4.5 floor is
// WCAG AA for normal text.
func TestThemeContrast(t *testing.T) {
	for _, hue := range accentNames {
		for _, tc := range []struct {
			name    string
			variant fyne.ThemeVariant
		}{
			{"light", theme.VariantLight},
			{"dark", theme.VariantDark},
		} {
			t.Run(hue+"_"+tc.name, func(t *testing.T) {
				gogg := &GoggTheme{Theme: theme.DefaultTheme(), variant: &tc.variant, accent: accentByName(hue)}
				pick := func(name fyne.ThemeColorName) color.Color {
					// The variant argument is the system's; the forced variant
					// must win regardless, so the opposite one is passed in.
					opposite := theme.VariantLight
					if tc.variant == theme.VariantLight {
						opposite = theme.VariantDark
					}
					return gogg.Color(name, opposite)
				}

				background := pick(theme.ColorNameBackground)
				foreground := pick(theme.ColorNameForeground)
				primary := pick(theme.ColorNamePrimary)
				onPrimary := pick(theme.ColorNameForegroundOnPrimary)
				hyperlink := pick(theme.ColorNameHyperlink)
				selection := pick(theme.ColorNameSelection)

				require.GreaterOrEqual(t, contrast(foreground, background), 4.5,
					"plain text on the plain background")
				require.GreaterOrEqual(t, contrast(onPrimary, primary), 4.5,
					"text on the accent, such as the download button's label")
				require.GreaterOrEqual(t, contrast(hyperlink, background), 4.5,
					"links on the plain background")
				require.GreaterOrEqual(t, contrast(foreground, over(selection, background)), 4.5,
					"text on a selected row, where the accent tint sits behind it")
			})
		}
	}
}

// Borders and separators are not text, but they still have to be seen: 3.0
// is the WCAG floor for component boundaries, and separators stay above a
// softer floor that keeps them visible without shouting.
func TestThemeBorders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		variant fyne.ThemeVariant
	}{
		{"light", theme.VariantLight},
		{"dark", theme.VariantDark},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gogg := &GoggTheme{Theme: theme.DefaultTheme(), variant: &tc.variant}
			background := gogg.Color(theme.ColorNameBackground, tc.variant)
			inputBackground := gogg.Color(theme.ColorNameInputBackground, tc.variant)
			border := gogg.Color(theme.ColorNameInputBorder, tc.variant)
			separator := gogg.Color(theme.ColorNameSeparator, tc.variant)

			require.GreaterOrEqual(t, contrast(border, background), 3.0,
				"an input's outline against the page")
			require.GreaterOrEqual(t, contrast(border, inputBackground), 3.0,
				"and against the input's own fill")
			require.GreaterOrEqual(t, contrast(separator, background), 1.6,
				"separators divide, so they have to be seen dividing")
		})
	}
}

// Picking a hue in the settings is what changes the accent.
func TestThemeAccentFollowsThePreference(t *testing.T) {
	blue := &GoggTheme{Theme: theme.DefaultTheme(), accent: accentByName("Blue")}
	purple := &GoggTheme{Theme: theme.DefaultTheme()}
	require.NotEqual(t,
		blue.Color(theme.ColorNamePrimary, theme.VariantLight),
		purple.Color(theme.ColorNamePrimary, theme.VariantLight))
	require.Equal(t, accentByName("Purple"), accentByName("no such hue"),
		"an unknown stored hue falls back to the default")
}

// The accent is purple in both variants, not the blue it used to be: more
// red than green in the channel mix, and blue ahead of both.
func TestThemeAccentIsPurple(t *testing.T) {
	for _, variant := range []fyne.ThemeVariant{theme.VariantLight, theme.VariantDark} {
		gogg := &GoggTheme{Theme: theme.DefaultTheme(), variant: &variant}
		r, g, b, _ := gogg.Color(theme.ColorNamePrimary, variant).RGBA()
		require.Greater(t, r, g, "purple carries more red than green")
		require.Greater(t, b, r, "and blue ahead of both")
	}
}

// The softened corners hold in both directions: rounder than the default,
// still far from a circle.
func TestThemeShapes(t *testing.T) {
	gogg := &GoggTheme{Theme: theme.DefaultTheme()}
	require.Equal(t, float32(8), gogg.Size(theme.SizeNameInputRadius))
	require.Equal(t, float32(6), gogg.Size(theme.SizeNameSelectionRadius))
	require.Equal(t, theme.DefaultTheme().Size(theme.SizeNamePadding), gogg.Size(theme.SizeNamePadding),
		"everything not named keeps the default")
}
