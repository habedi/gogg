package cmd

import "strings"

// gameLanguages is a map that associates language codes with their native names.
// The keys are language codes (e.g., "en" for English) and the values are the native names of the languages.
var gameLanguages = map[string]string{
	"en":      "English",
	"fr":      "Français",            // French
	"de":      "Deutsch",             // German
	"es":      "Español",             // Spanish
	"it":      "Italiano",            // Italian
	"ru":      "Русский",             // Russian
	"pl":      "Polski",              // Polish
	"pt-BR":   "Português do Brasil", // Portuguese (Brazil)
	"zh-Hans": "简体中文",                // Simplified Chinese
	"ja":      "日本語",                 // Japanese
	"ko":      "한국어",                 // Korean
}

// isValidLanguage checks if a given language code is valid.
// It returns true if the language code exists in the gameLanguages map, otherwise false.
func isValidLanguage(lang string) bool {
	lang = strings.ToLower(lang)
	_, ok := gameLanguages[lang]
	return ok
}
