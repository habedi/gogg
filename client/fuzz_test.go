//go:build go1.18

package client

import "testing"

func FuzzParseSizeString(f *testing.F) {
	seed := []string{"1", "1024", "1KB", "1 KB", "1.5MB", "2 mb", "10GB", "0.1 gb", "3TB", "1GiB", "invalid"}
	for _, s := range seed {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = parseSizeString(s)
	})
}

func FuzzParseGameData(f *testing.F) {
	seed := []string{
		`{"title":"Test","downloads":[],"extras":[],"dlcs":[]}`,
		`{"title":"","downloads":[],"extras":[],"dlcs":[]}`,
	}
	for _, s := range seed {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ParseGameData(s)
	})
}
