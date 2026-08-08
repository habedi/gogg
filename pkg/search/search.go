// Package search decides which games the library lists. The search box, the
// filter dialog, and the sidebar rows all turn what they know into a query and
// ask it, so there is one place that decides what "downloaded" means.
package search

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Facts are what gogg knows about a game when it decides whether to list it.
// Anything the library cannot answer cheaply stays out: a query is asked for
// every game on every keystroke.
type Facts struct {
	Title      string
	Platforms  []string // windows, mac, or linux
	Languages  []string // language codes, as stored in the catalogue
	Downloaded bool
	HasUpdate  bool
	SizeBytes  int64
	Tags       []string // what the user has marked the game with
	Genres     []string // what GOG's store says the game is, once looked up
	// UpdatedAt is when gogg first noticed the update now waiting for the
	// game; zero when none is.
	UpdatedAt time.Time
}

// Query is a parsed search. An empty one matches every game.
type Query struct {
	terms []term
}

// term is one condition. Terms are joined with and: every one has to match, the
// way a search box is expected to narrow as you type.
type term struct {
	field string // empty for free text, which matches the title
	value string
	// flag is what a yes or no field was set to.
	flag bool
	// operator and size are how a size is compared, ">=" by default. when is
	// the moment an updated term compares against, under the same operator:
	// updated:>30d means noticed within the last thirty days.
	operator string
	size     int64
	when     time.Time
}

// Parse reads a query. Free words match the title; the rest are field:value,
// which is what makes a sidebar row and a typed search the same thing.
//
//	god of war                 the title contains all of these words
//	downloaded:no updates:yes  games worth fetching again
//	platform:linux lang:de     what a game offers
//	size:>10gb                 also <, >=, <=, and a bare size meaning >=
//	tag:favourite hidden:no    what the user marked
func Parse(input string) (Query, error) {
	var query Query
	for _, token := range tokenize(input) {
		parsed, err := parseTerm(token)
		if err != nil {
			return Query{}, err
		}
		query.terms = append(query.terms, parsed)
	}
	return query, nil
}

// Needs says which facts a query looks at. Finding a game's size or the
// platforms it offers means parsing its stored data, which is worth skipping for
// the queries that never ask.
type Needs struct {
	Platforms bool
	Languages bool
	Size      bool
	Tags      bool
	Genres    bool
}

// Needs reports what the library has to work out before asking this query.
func (q Query) Needs() Needs {
	var needs Needs
	for _, t := range q.terms {
		switch t.field {
		case "platform":
			needs.Platforms = true
		case "lang":
			needs.Languages = true
		case "size":
			needs.Size = true
		case "tag", "favorite", "hidden":
			needs.Tags = true
		case "genre":
			needs.Genres = true
		}
	}
	return needs
}

// HasTerm reports whether a query already asks for a yes-or-no field. The
// filter dialog reads it to show what the current search is doing.
func (q Query) HasTerm(field, value string) bool {
	want, err := parseFlag(value)
	if err != nil {
		return false
	}
	for _, t := range q.terms {
		if t.field == resolveAlias(field) && t.flag == want {
			return true
		}
	}
	return false
}

// TermValue returns what a compared field was set to, as it would be typed
// again, or empty when the query does not mention it.
func (q Query) TermValue(field, operator string) string {
	for _, t := range q.terms {
		if t.field != resolveAlias(field) || t.operator != operator {
			continue
		}
		if t.field == "size" {
			return FormatSize(t.size)
		}
		return t.value
	}
	return ""
}

// FormatSize writes a size the way a query takes it back, in the units the app
// shows beside a game.
func FormatSize(size int64) string {
	switch {
	case size >= 1<<40:
		return trimZeros(float64(size)/(1<<40)) + "tib"
	case size >= 1<<30:
		return trimZeros(float64(size)/(1<<30)) + "gib"
	case size >= 1<<20:
		return trimZeros(float64(size)/(1<<20)) + "mib"
	default:
		return strconv.FormatInt(size, 10)
	}
}

func trimZeros(value float64) string {
	text := strconv.FormatFloat(value, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	return strings.TrimRight(text, ".")
}

// Words returns the free text of a query without its field terms. The filter
// dialog rewrites the fields it owns and leaves what the user typed alone.
func Words(input string) string {
	var words []string
	for _, token := range tokenize(input) {
		field, _, isField := strings.Cut(token, ":")
		field = strings.ToLower(field)
		if isField && (flagFields[field] || valueFields[field] || knownAlias(field)) {
			continue
		}
		words = append(words, token)
	}
	return strings.Join(words, " ")
}

// Mentions reports whether a query says anything about a field. The library
// asks before it adds a term of its own, so a search that is about hidden games
// is left to say so itself.
func (q Query) Mentions(field string) bool {
	field = resolveAlias(field)
	for _, t := range q.terms {
		if t.field == field {
			return true
		}
	}
	return false
}

// And narrows a query with another one's terms.
func (q Query) And(other Query) Query {
	return Query{terms: append(append([]term{}, q.terms...), other.terms...)}
}

// IsEmpty reports whether the query lets everything through.
func (q Query) IsEmpty() bool { return len(q.terms) == 0 }

// Match reports whether a game belongs in the list.
func (q Query) Match(facts Facts) bool {
	for _, t := range q.terms {
		if !t.match(facts) {
			return false
		}
	}
	return true
}

func (t term) match(facts Facts) bool {
	switch t.field {
	case "", "title":
		return strings.Contains(strings.ToLower(facts.Title), t.value)
	case "downloaded":
		return facts.Downloaded == t.flag
	case "updates":
		return facts.HasUpdate == t.flag
	case "platform":
		return containsFold(facts.Platforms, t.value)
	case "lang":
		return containsFold(facts.Languages, t.value)
	case "tag":
		return containsFold(facts.Tags, t.value)
	case "genre":
		// A substring, because GOG's names are long: genre:role finds
		// "Role-playing" without anyone typing the hyphen.
		return anyContainsFold(facts.Genres, t.value)
	case "favorite", "hidden":
		return containsFold(facts.Tags, t.field) == t.flag
	case "size":
		return t.matchSize(facts.SizeBytes)
	case "updated":
		return t.matchUpdated(facts.UpdatedAt)
	}
	return false
}

// matchUpdated compares when an update was noticed against the term's moment.
// ">" reads as "more recently than": updated:>30d is the last thirty days.
func (t term) matchUpdated(noticed time.Time) bool {
	if noticed.IsZero() {
		return false
	}
	switch t.operator {
	case "<", "<=":
		return noticed.Before(t.when)
	default:
		return !noticed.Before(t.when)
	}
}

func (t term) matchSize(size int64) bool {
	switch t.operator {
	case "<":
		return size < t.size
	case "<=":
		return size <= t.size
	case ">":
		return size > t.size
	default:
		return size >= t.size
	}
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(value, want) {
			return true
		}
	}
	return false
}

func anyContainsFold(values []string, want string) bool {
	want = strings.ToLower(want)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), want) {
			return true
		}
	}
	return false
}

// flagFields are the yes-or-no fields; valueFields take something to compare.
var (
	flagFields  = map[string]bool{"downloaded": true, "updates": true, "favorite": true, "hidden": true}
	valueFields = map[string]bool{
		"title": true, "platform": true, "lang": true, "tag": true,
		"size": true, "genre": true, "updated": true,
	}
)

func parseTerm(token string) (term, error) {
	field, value, isField := strings.Cut(token, ":")
	field = strings.ToLower(field)
	if !isField || (!flagFields[field] && !valueFields[field] && !knownAlias(field)) {
		// Anything that is not a field is a word from the title. A colon in a
		// game's name is not a filter.
		return term{value: strings.ToLower(unquote(token))}, nil
	}

	field = resolveAlias(field)
	value = unquote(strings.TrimSpace(value))
	if value == "" {
		return term{}, fmt.Errorf("%q needs something after the colon", field)
	}

	if flagFields[field] {
		flag, err := parseFlag(value)
		if err != nil {
			return term{}, fmt.Errorf("%q takes yes or no, not %q", field, value)
		}
		return term{field: field, flag: flag}, nil
	}

	if field == "size" {
		operator, rest := splitOperator(value)
		size, err := ParseSize(rest)
		if err != nil {
			return term{}, fmt.Errorf("size %q is not a size such as 10gb", value)
		}
		return term{field: field, operator: operator, size: size}, nil
	}

	if field == "updated" {
		operator, rest := splitOperator(value)
		when, err := parseSince(rest)
		if err != nil {
			return term{}, fmt.Errorf("updated %q is not a date such as 2026-01-31 or an age such as 30d", value)
		}
		return term{field: field, operator: operator, when: when, value: rest}, nil
	}

	return term{field: field, value: strings.ToLower(value)}, nil
}

// now is read rather than called so tests can hold the clock still.
var now = time.Now

// sinceUnits are the ages a query can name: days, weeks, months, and years,
// the way a person says "in the last month".
var sinceUnits = map[byte]time.Duration{
	'd': 24 * time.Hour,
	'w': 7 * 24 * time.Hour,
	'm': 30 * 24 * time.Hour,
	'y': 365 * 24 * time.Hour,
}

// parseSince reads a moment, either as a date or as an age: updated:>30d is
// the last thirty days, updated:>2026-01-31 is since that day.
func parseSince(value string) (time.Time, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) >= 2 {
		if unit, ok := sinceUnits[value[len(value)-1]]; ok {
			count, err := strconv.Atoi(value[:len(value)-1])
			if err == nil && count >= 0 {
				return now().Add(-time.Duration(count) * unit), nil
			}
		}
	}
	when, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, err
	}
	return when, nil
}

// aliases let a query read the way it is spoken.
var aliases = map[string]string{
	"update":    "updates",
	"language":  "lang",
	"favourite": "favorite",
	"installed": "downloaded",
	"genres":    "genre",
}

func knownAlias(field string) bool { _, ok := aliases[field]; return ok }

func resolveAlias(field string) string {
	if resolved, ok := aliases[field]; ok {
		return resolved
	}
	return field
}

func parseFlag(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "yes", "y", "true", "1", "on":
		return true, nil
	case "no", "n", "false", "0", "off":
		return false, nil
	}
	return false, fmt.Errorf("not a yes or no")
}

func splitOperator(value string) (string, string) {
	for _, operator := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(value, operator) {
			return operator, strings.TrimSpace(value[len(operator):])
		}
	}
	return ">=", value
}

// ParseSize reads a size such as 10gb, 1.5 TB, or 512mb. A number on its own is
// a count of bytes.
func ParseSize(input string) (int64, error) {
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return 0, fmt.Errorf("no size given")
	}

	multiplier := int64(1)
	// Longest first: "gib" has to be recognised before the "b" at the end of it
	// leads anywhere else. Sizes are powers of two whichever way they are
	// written, which is what the app shows beside a game.
	for _, unit := range []struct {
		suffix string
		scale  int64
	}{
		{"tib", 1 << 40}, {"gib", 1 << 30}, {"mib", 1 << 20}, {"kib", 1 << 10},
		{"tb", 1 << 40}, {"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10},
		{"t", 1 << 40}, {"g", 1 << 30}, {"m", 1 << 20}, {"k", 1 << 10},
	} {
		if strings.HasSuffix(text, unit.suffix) {
			multiplier = unit.scale
			text = strings.TrimSpace(strings.TrimSuffix(text, unit.suffix))
			break
		}
	}

	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%q is not a size", input)
	}
	return int64(math.Round(value * float64(multiplier))), nil
}

// tokenize splits on spaces, keeping quoted runs together so a title with
// spaces can be asked for.
func tokenize(input string) []string {
	var (
		tokens  []string
		current strings.Builder
		quoted  bool
	)
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}

	for _, r := range input {
		switch {
		case r == '"':
			quoted = !quoted
			current.WriteRune(r)
		case r == ' ' && !quoted:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func unquote(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value[1 : len(value)-1]
	}
	return strings.ReplaceAll(value, `"`, "")
}
