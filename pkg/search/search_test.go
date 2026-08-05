package search

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func facts() Facts {
	return Facts{
		Title:      "God of War",
		Platforms:  []string{"windows", "linux"},
		Languages:  []string{"en", "de"},
		Downloaded: true,
		HasUpdate:  false,
		SizeBytes:  40 << 30,
		Tags:       []string{"favorite"},
	}
}

func matches(t *testing.T, query string, subject Facts) bool {
	t.Helper()
	parsed, err := Parse(query)
	require.NoError(t, err)
	return parsed.Match(subject)
}

// An empty search lists the whole library.
func TestQuery_EmptyMatchesEverything(t *testing.T) {
	parsed, err := Parse("   ")
	require.NoError(t, err)
	require.True(t, parsed.IsEmpty())
	require.True(t, parsed.Match(Facts{}))
}

// Words match the title, whatever case they are typed in.
func TestQuery_WordsMatchTheTitle(t *testing.T) {
	require.True(t, matches(t, "god", facts()))
	require.True(t, matches(t, "GOD", facts()))
	require.True(t, matches(t, "war", facts()))
	require.False(t, matches(t, "hollow", facts()))
}

// Terms narrow: every one of them has to match.
func TestQuery_TermsAreJoinedWithAnd(t *testing.T) {
	require.True(t, matches(t, "god downloaded:yes", facts()))
	require.False(t, matches(t, "god downloaded:no", facts()))
	require.False(t, matches(t, "hollow downloaded:yes", facts()))
}

func TestQuery_Fields(t *testing.T) {
	subject := facts()

	require.True(t, matches(t, "downloaded:yes", subject))
	require.False(t, matches(t, "downloaded:no", subject))
	require.True(t, matches(t, "updates:no", subject))
	require.False(t, matches(t, "updates:yes", subject))

	require.True(t, matches(t, "platform:linux", subject))
	require.True(t, matches(t, "platform:Linux", subject), "platforms are not case sensitive")
	require.False(t, matches(t, "platform:mac", subject))

	require.True(t, matches(t, "lang:de", subject))
	require.False(t, matches(t, "lang:fr", subject))

	require.True(t, matches(t, "tag:favorite", subject))
	require.True(t, matches(t, "favorite:yes", subject))
	require.False(t, matches(t, "favorite:no", subject))
	require.True(t, matches(t, "hidden:no", subject), "a game with no hidden tag is not hidden")
}

// The words a user reaches for should work too.
func TestQuery_Aliases(t *testing.T) {
	subject := facts()
	require.True(t, matches(t, "installed:yes", subject))
	require.True(t, matches(t, "language:en", subject))
	require.True(t, matches(t, "update:no", subject))
	require.True(t, matches(t, "favourite:yes", subject))
}

func TestQuery_SizeComparisons(t *testing.T) {
	subject := facts() // 40 GiB

	require.True(t, matches(t, "size:>10gb", subject))
	require.False(t, matches(t, "size:>100gb", subject))
	require.True(t, matches(t, "size:<50gb", subject))
	require.True(t, matches(t, "size:>=40gb", subject))
	require.True(t, matches(t, "size:<=40gb", subject))
	require.True(t, matches(t, "size:10gb", subject), "a bare size asks for at least that much")
	require.True(t, matches(t, "size:>1.5gb", subject))
}

// A title with a colon in it is a title, not a filter nobody has heard of.
func TestQuery_UnknownFieldIsReadAsAWord(t *testing.T) {
	subject := Facts{Title: "Half-Life 2: Episode One"}
	require.True(t, matches(t, "episode", subject))
	require.True(t, matches(t, "2:", subject))
}

// A field with nothing after it cannot be answered, and saying so beats
// listing nothing without explanation.
func TestParse_ReportsWhatItCannotRead(t *testing.T) {
	for _, query := range []string{"downloaded:", "downloaded:maybe", "size:>lots", "platform:"} {
		_, err := Parse(query)
		require.Error(t, err, "%q should not parse", query)
	}
}

// Quoted words stay together, which is the only way to search a title with a
// space in it alongside another term.
func TestQuery_QuotedValues(t *testing.T) {
	subject := facts()
	require.True(t, matches(t, `title:"god of war"`, subject))
	require.False(t, matches(t, `title:"god of peace"`, subject))
	require.True(t, matches(t, `title:"god of war" platform:linux`, subject))
}

func TestParseSize(t *testing.T) {
	for input, want := range map[string]int64{
		"10gb":   10 << 30,
		"10 GB":  10 << 30,
		"512mb":  512 << 20,
		"1.5tb":  1<<40 + 1<<39,
		"2g":     2 << 30,
		"1024":   1024,
		"0":      0,
		"1.5 kb": 1536,
	} {
		got, err := ParseSize(input)
		require.NoError(t, err, input)
		require.Equal(t, want, got, input)
	}

	for _, input := range []string{"", "lots", "-5gb", "gb"} {
		_, err := ParseSize(input)
		require.Error(t, err, input)
	}
}

// Working out a game's size means parsing its stored data, so a query that
// never mentions size should not pay for it.
func TestQuery_NeedsOnlyWhatItAsksAbout(t *testing.T) {
	plain, err := Parse("god of war downloaded:yes")
	require.NoError(t, err)
	require.Equal(t, Needs{}, plain.Needs())

	everything, err := Parse("size:>1gb platform:linux lang:de tag:rpg")
	require.NoError(t, err)
	require.Equal(t, Needs{Platforms: true, Languages: true, Size: true, Tags: true}, everything.Needs())

	marked, err := Parse("hidden:no")
	require.NoError(t, err)
	require.True(t, marked.Needs().Tags)
}

// The filter dialog owns the field terms and leaves the typed words alone.
func TestWords(t *testing.T) {
	require.Equal(t, "god of war", Words("god downloaded:yes of size:>1gb war"))
	require.Equal(t, "", Words("downloaded:yes updates:no"))
	require.Equal(t, `"god of war"`, Words(`"god of war" platform:linux`))
	require.Equal(t, "half-life 2:", Words("half-life 2:"))
}

// The filter dialog opens showing what the search box already says.
func TestQuery_ReadsBackItsOwnTerms(t *testing.T) {
	parsed, err := Parse("god downloaded:yes size:>=10gb size:<=1.5tb")
	require.NoError(t, err)

	require.True(t, parsed.HasTerm("downloaded", "yes"))
	require.False(t, parsed.HasTerm("downloaded", "no"))
	require.False(t, parsed.HasTerm("updates", "yes"), "a term that is not there")

	require.Equal(t, "10gb", parsed.TermValue("size", ">="))
	require.Equal(t, "1.5tb", parsed.TermValue("size", "<="))
	require.Empty(t, parsed.TermValue("size", "<"))
}

// What a query prints has to parse back to the same thing.
func TestFormatSize_RoundTrips(t *testing.T) {
	for _, size := range []int64{512, 1 << 20, 10 << 30, 1<<40 + 1<<39} {
		again, err := ParseSize(FormatSize(size))
		require.NoError(t, err)
		require.Equal(t, size, again, FormatSize(size))
	}
}
