package gui

import (
	"slices"
	"sort"
	"strings"

	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
)

// languageCodes maps a language's display name back to the code stored in
// preferences. Names are what GOG puts in the game data, codes are what the
// rest of gogg works with.
var languageCodes = func() map[string]string {
	codes := make(map[string]string, len(client.GameLanguages))
	for code, name := range client.GameLanguages {
		codes[name] = code
	}
	return codes
}()

// languageChoices returns the language names to offer for a game. Languages the
// game does not ship in are left out. GOG publishes in more languages than gogg
// has codes for, so when none of a game's languages are known, everything is
// offered rather than nothing.
func languageChoices(offered []string) []string {
	known := make([]string, 0, len(offered))
	for _, name := range offered {
		if _, ok := languageCodes[name]; ok {
			known = append(known, name)
		}
	}
	if len(known) == 0 {
		known = allLanguageNames()
	}
	sort.Strings(known)
	return known
}

func allLanguageNames() []string {
	names := make([]string, 0, len(client.GameLanguages))
	for _, name := range client.GameLanguages {
		names = append(names, name)
	}
	return names
}

// platformChoices returns the platform values to offer for a game. "all" is
// always on offer because it means "whatever this game has".
func platformChoices(offered []string) []string {
	if len(offered) == 0 {
		return []string{"windows", "mac", "linux", "all"}
	}

	choices := make([]string, 0, len(offered)+1)
	for _, platform := range offered {
		choices = append(choices, strings.ToLower(platform))
	}
	return append(choices, "all")
}

// bindSelect points a select at a new set of options. The change handler is
// detached first: narrowing the list to suit a game is not the user choosing,
// and must not overwrite the preference they set themselves. A wanted value the
// game does not offer falls back to the first one it does.
func bindSelect(sel *widget.Select, options []string, wanted string, onChanged func(string)) {
	sel.OnChanged = nil
	sel.Options = options
	if !slices.Contains(options, wanted) && len(options) > 0 {
		wanted = options[0]
	}
	sel.SetSelected(wanted)
	sel.OnChanged = onChanged
	sel.Refresh()
}

// platformGroupChoices is platformChoices without "all": in a set of check
// boxes, all is every box ticked.
func platformGroupChoices(offered []string) []string {
	choices := platformChoices(offered)
	return choices[:len(choices)-1]
}

// bindCheckGroup points a check group at a new set of options, keeping of the
// wanted values the ones the game offers. The change handler is detached
// first, for the reason bindSelect gives. When nothing wanted is on offer,
// the first option is ticked: a download needs at least one of everything.
func bindCheckGroup(group *widget.CheckGroup, options, wanted []string, onChanged func([]string)) {
	group.OnChanged = nil
	group.Options = options
	kept := make([]string, 0, len(wanted))
	for _, want := range wanted {
		if slices.Contains(options, want) {
			kept = append(kept, want)
		}
	}
	if len(kept) == 0 && len(options) > 0 {
		kept = options[:1]
	}
	group.SetSelected(kept)
	group.OnChanged = onChanged
	group.Refresh()
}

// languageNamesFor turns stored comma-joined codes back into display names,
// dropping codes gogg no longer knows.
func languageNamesFor(codes string) []string {
	names := make([]string, 0, 2)
	for _, code := range splitCSV(codes) {
		if name, ok := client.GameLanguages[code]; ok {
			names = append(names, name)
		}
	}
	return names
}

// splitCSV reads a comma-joined preference back into its values.
func splitCSV(joined string) []string {
	values := make([]string, 0, 2)
	for _, part := range strings.Split(joined, ",") {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}
