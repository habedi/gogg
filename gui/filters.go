package gui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/pkg/search"
)

// anyChoice is what a select says when it is not filtering on anything.
const anyChoice = "Any"

// filterChoices is what the filter dialog was set to.
type filterChoices struct {
	Downloaded, HasUpdate bool
	MinSize, MaxSize      string
	Platform, Language    string
	Tag, Genre            string
	// UpdatedSince narrows to updates noticed within an age or since a date.
	UpdatedSince string
}

// filterTerms turns what the filter dialog was set to into the terms it writes
// into the search box, so the dialog and the box stay the same filter.
func filterTerms(choices filterChoices) []string {
	var terms []string
	if choices.Downloaded {
		terms = append(terms, "downloaded:yes")
	}
	if choices.HasUpdate {
		terms = append(terms, "updates:yes")
	}
	if size := strings.ReplaceAll(strings.TrimSpace(choices.MinSize), " ", ""); size != "" {
		terms = append(terms, "size:>="+size)
	}
	if size := strings.ReplaceAll(strings.TrimSpace(choices.MaxSize), " ", ""); size != "" {
		terms = append(terms, "size:<="+size)
	}
	for _, field := range []struct{ name, chosen string }{
		{"platform", choices.Platform}, {"lang", choices.Language},
		{"tag", choices.Tag}, {"genre", choices.Genre},
	} {
		if value := strings.TrimSpace(field.chosen); value != "" && value != anyChoice {
			terms = append(terms, field.name+":"+value)
		}
	}
	if since := strings.ReplaceAll(strings.TrimSpace(choices.UpdatedSince), " ", ""); since != "" {
		terms = append(terms, "updated:>="+since)
	}
	return terms
}

// withoutHidden leaves out the games the user has marked hidden, unless the
// query is about hidden games. The tag put them in a collection of their own
// and left them in every other list as well, which is not what hiding means.
func withoutHidden(query search.Query) search.Query {
	if query.Mentions("hidden") {
		return query
	}
	notHidden, err := search.Parse("hidden:no")
	if err != nil {
		return query
	}
	return query.And(notHidden)
}

// newFiltersButton edits the field terms of the search, leaving the words the
// user typed. The dialog and the search box are then the same filter, one of
// them just easier to discover.
func newFiltersButton(searchEntry *widget.Entry, refresh func()) *widget.Button {
	var dlg *dialog.CustomDialog
	btn := widget.NewButtonWithIcon("Filters", theme.SearchIcon(), func() {
		current, _ := search.Parse(searchEntry.Text)

		downloadedChk := widget.NewCheck("Downloaded only", nil)
		downloadedChk.SetChecked(current.HasTerm("downloaded", "yes"))
		updateChk := widget.NewCheck("Has update", nil)
		updateChk.SetChecked(current.HasTerm("updates", "yes"))

		// The sizes are suggested the way the app writes them, so what the
		// dialog hints at is what a game's size beside it says.
		sizeMinEntry := widget.NewEntry()
		sizeMinEntry.SetPlaceHolder("10 GiB")
		sizeMinEntry.SetText(current.TermValue("size", ">="))
		sizeMaxEntry := widget.NewEntry()
		sizeMaxEntry.SetPlaceHolder("50 GiB")
		sizeMaxEntry.SetText(current.TermValue("size", "<="))

		// The box understands more than downloads and sizes, so the dialog
		// offers the rest of it rather than half. Platforms and languages
		// show the names people know them by, while the terms they write use
		// the keys and codes the search matches on.
		platformSelect := widget.NewSelect([]string{anyChoice, "Windows", "macOS", "Linux"}, nil)
		platformSelect.SetSelected(chosenOr(platformInWords(current.TermValue("platform", "")), anyChoice))
		languageSelect := widget.NewSelect(append([]string{anyChoice}, languageChoices(nil)...), nil)
		languageSelect.SetSelected(chosenOr(languageNameForCode(current.TermValue("lang", "")), anyChoice))
		tagEntry := widget.NewEntry()
		tagEntry.SetPlaceHolder("finished")
		tagEntry.SetText(current.TermValue("tag", ""))
		genreEntry := widget.NewEntry()
		genreEntry.SetPlaceHolder("strategy")
		genreEntry.SetText(current.TermValue("genre", ""))
		updatedEntry := widget.NewEntry()
		updatedEntry.SetPlaceHolder("30d, or 2026-01-31")
		updatedEntry.SetText(current.TermValue("updated", ">="))

		apply := func(terms ...string) {
			query := strings.TrimSpace(strings.Join(append([]string{search.Words(searchEntry.Text)}, terms...), " "))
			searchEntry.SetText(query)
			refresh()
			dlg.Hide()
		}

		applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
			apply(filterTerms(filterChoices{
				Downloaded: downloadedChk.Checked, HasUpdate: updateChk.Checked,
				MinSize: sizeMinEntry.Text, MaxSize: sizeMaxEntry.Text,
				// The names on screen become the keys and codes the search
				// terms carry.
				Platform: platformKeyOrAny(platformSelect.Selected),
				Language: languageCodeOrAny(languageSelect.Selected),
				Tag:      tagEntry.Text, Genre: genreEntry.Text,
				UpdatedSince: updatedEntry.Text,
			})...)
		})
		resetBtn := widget.NewButtonWithIcon("Reset", theme.ViewRefreshIcon(), func() { apply() })

		content := container.NewVBox(
			widget.NewForm(
				widget.NewFormItem("Min Size", sizeMinEntry),
				widget.NewFormItem("Max Size", sizeMaxEntry),
				widget.NewFormItem("Platform", platformSelect),
				widget.NewFormItem("Language", languageSelect),
				widget.NewFormItem("Tag", tagEntry),
				widget.NewFormItem("Genre", genreEntry),
				widget.NewFormItem("Updated in", updatedEntry),
			),
			container.NewGridWithColumns(2, downloadedChk, updateChk),
			widget.NewLabel("These become terms in the search box, where they can also be typed."),
			container.NewHBox(applyBtn, resetBtn),
		)
		dlg = dialog.NewCustom("Library Filters", "Close", content, fyne.CurrentApp().Driver().AllWindows()[0])
		dlg.Resize(fyne.NewSize(460, 420))
		dlg.Show()
	})
	return btn
}

// chosenOr is what a select shows: what the search says, or that it is not
// filtering on this at all.
func chosenOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// languageNameForCode is the full name of a stored language code, empty when
// there is no code to name so the select falls back to "Any".
func languageNameForCode(code string) string {
	if code == "" {
		return ""
	}
	if name, ok := client.GameLanguages[code]; ok {
		return name
	}
	return code
}

// platformKeyOrAny maps a platform label back to its key, leaving "Any"
// alone so filterTerms drops it.
func platformKeyOrAny(label string) string {
	if label == anyChoice || label == "" {
		return label
	}
	return platformKeyFor(label)
}

// languageCodeOrAny maps a language name back to its code, leaving "Any"
// alone so filterTerms drops it.
func languageCodeOrAny(name string) string {
	if name == anyChoice || name == "" {
		return name
	}
	if code, ok := languageCodes[name]; ok {
		return code
	}
	return name
}
