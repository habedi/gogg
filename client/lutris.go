package client

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Lutris caches installer files under <cache>/<game-slug>/gog/, where the
// slug is Lutris's own slugify of the game name. Reproducing it exactly is
// the point: a folder that differs by one character is a cache miss and a
// re-download for Lutris.

var (
	lutrisNonWord    = regexp.MustCompile(`[^\w\s-]`)
	lutrisSeparators = regexp.MustCompile(`[-\s]+`)
)

// LutrisSlug names a game the way Lutris does: accents folded away, anything
// that is not a word character dropped, and runs of spaces and hyphens
// collapsed to one hyphen. A name with nothing left, such as one written
// entirely in a non-latin script, becomes the same UUID Lutris would use.
func LutrisSlug(title string) string {
	folded := make([]rune, 0, len(title))
	for _, r := range norm.NFD.String(title) {
		if r < unicode.MaxASCII {
			folded = append(folded, r)
		}
	}

	cleaned := lutrisNonWord.ReplaceAllString(string(folded), "")
	cleaned = strings.ToLower(strings.TrimSpace(cleaned))
	slug := lutrisSeparators.ReplaceAllString(cleaned, "-")
	if slug == "" {
		return uuid5URL(title)
	}
	return slug
}

// uuid5URL is a version-5 UUID in the URL namespace, the fallback Lutris
// uses for names that slugify to nothing.
func uuid5URL(name string) string {
	namespaceURL := []byte{
		0x6b, 0xa7, 0xb8, 0x11, 0x9d, 0xad, 0x11, 0xd1,
		0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
	}
	sum := sha1.Sum(append(namespaceURL, []byte(name)...))

	var id [16]byte
	copy(id[:], sum[:16])
	id[6] = (id[6] & 0x0f) | 0x50 // version 5
	id[8] = (id[8] & 0x3f) | 0x80 // RFC 4122 variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[0:4], id[4:6], id[6:8], id[8:10], id[10:16])
}
