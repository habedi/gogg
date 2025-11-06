package client

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"
)

func randString(r *rand.Rand, n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 -_()[]{}")
	runes := make([]rune, n)
	for i := range runes {
		runes[i] = letters[r.Intn(len(letters))]
	}
	return string(runes)
}

func randPlatformFile(r *rand.Rand) PlatformFile {
	var mu *string
	if r.Intn(2) == 0 {
		s := "/download/" + randString(r, 8) + ".bin"
		mu = &s
	}
	var ver *string
	if r.Intn(2) == 0 {
		v := randString(r, 3)
		ver = &v
	}
	var dt *string
	if r.Intn(2) == 0 {
		d := time.Now().Add(time.Duration(r.Intn(1000)) * time.Hour).Format(time.RFC3339)
		dt = &d
	}
	return PlatformFile{
		ManualURL: mu,
		Name:      randString(r, 6) + ".exe",
		Version:   ver,
		Date:      dt,
		Size:      []string{"1 GB", "2 GB", "50 MB", "1024"}[r.Intn(4)],
	}
}

func randDownloadable(r *rand.Rand) []interface{} {
	lang := []string{"English", "Français", "Deutsch"}[r.Intn(3)]
	// Vary counts per platform aggressively
	win := make([]PlatformFile, r.Intn(4))
	for i := range win {
		win[i] = randPlatformFile(r)
	}
	mac := make([]PlatformFile, r.Intn(4))
	for i := range mac {
		mac[i] = randPlatformFile(r)
	}
	lin := make([]PlatformFile, r.Intn(4))
	for i := range lin {
		lin[i] = randPlatformFile(r)
	}
	platforms := Platform{Windows: win, Mac: mac, Linux: lin}
	return []interface{}{lang, platforms}
}

func randExtra(r *rand.Rand) Extra {
	sizes := []string{"10 MB", "20 MB", "1 GB", "2 GB"}
	return Extra{Name: randString(r, 5), Size: sizes[r.Intn(len(sizes))], ManualURL: "/extras/" + randString(r, 6)}
}

func randDLC(r *rand.Rand) DLC {
	dl := make([][]interface{}, r.Intn(4))
	for i := range dl {
		dl[i] = randDownloadable(r)
	}
	extraCount := r.Intn(4)
	extras := make([]Extra, extraCount)
	for i := range extras {
		extras[i] = randExtra(r)
	}
	return DLC{Title: randString(r, 10), Downloads: dl, Extras: extras}
}

// Property-like test: generate many valid JSON structures and ensure our Unmarshal succeeds and invariants hold.
func TestProperty_UnmarshalGameJSON(t *testing.T) {
	r := rand.New(rand.NewSource(1234))
	for i := 0; i < 200; i++ {
		rawDownloads := make([][]interface{}, r.Intn(4))
		for j := range rawDownloads {
			rawDownloads[j] = randDownloadable(r)
		}
		extraCount := r.Intn(4)
		extras := make([]Extra, extraCount)
		for i := range extras {
			extras[i] = randExtra(r)
		}
		dlcs := make([]DLC, r.Intn(3))
		for i := range dlcs {
			dlcs[i] = randDLC(r)
		}
		gameJSON := map[string]interface{}{
			"title":     randString(r, 12),
			"downloads": rawDownloads,
			"extras":    extras,
			"dlcs":      dlcs,
		}
		b, err := json.Marshal(gameJSON)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var g Game
		if err := json.Unmarshal(b, &g); err != nil {
			t.Fatalf("unmarshal failed: %v\njson: %s", err, string(b))
		}
		// Invariants: downloads languages preserved, DLC ParsedDownloads length equals raw length
		for _, d := range g.Downloads {
			if d.Language == "" {
				t.Fatalf("missing language in parsed downloads: %s", string(b))
			}
		}
		for idx, dlc := range g.DLCs {
			if len(dlc.Downloads) != len(g.DLCs[idx].ParsedDownloads) {
				t.Fatalf("dlc parsed downloads mismatch: raw=%d parsed=%d", len(dlc.Downloads), len(g.DLCs[idx].ParsedDownloads))
			}
		}
	}
}

// New property-like test: For randomized game data, the size estimate should match
// an oracle computed from the same data for various flags.
func TestProperty_EstimateStorageSizeMatches(t *testing.T) {
	r := rand.New(rand.NewSource(4321))
	languages := []string{"English", "Français", "Deutsch"}
	platforms := []string{"windows", "mac", "linux", "all"}

	for iter := 0; iter < 150; iter++ {
		// Build random JSON
		rawDownloads := make([][]interface{}, r.Intn(4))
		for j := range rawDownloads {
			rawDownloads[j] = randDownloadable(r)
		}
		extraCount := r.Intn(4)
		extras := make([]Extra, extraCount)
		for i := range extras {
			extras[i] = randExtra(r)
		}
		dlcs := make([]DLC, r.Intn(3))
		for i := range dlcs {
			dlcs[i] = randDLC(r)
		}
		gameJSON := map[string]interface{}{
			"title":     "Sizer-" + randString(r, 6),
			"downloads": rawDownloads,
			"extras":    extras,
			"dlcs":      dlcs,
		}
		b, err := json.Marshal(gameJSON)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var g Game
		if err := json.Unmarshal(b, &g); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		// Pick parameters
		lang := languages[r.Intn(len(languages))]
		plat := platforms[r.Intn(len(platforms))]
		extraOn := r.Intn(2) == 0
		dlcOn := r.Intn(2) == 0

		// Compute expected using the same parse helper
		expected := int64(0)
		addFiles := func(files []PlatformFile) {
			for _, f := range files {
				if sz, err := parseSizeString(f.Size); err == nil {
					expected += sz
				}
			}
		}
		for _, d := range g.Downloads {
			if stringsEqualFold(d.Language, lang) {
				switch stringsLower(plat) {
				case "all":
					addFiles(d.Platforms.Windows)
					addFiles(d.Platforms.Mac)
					addFiles(d.Platforms.Linux)
				case "windows":
					addFiles(d.Platforms.Windows)
				case "mac":
					addFiles(d.Platforms.Mac)
				case "linux":
					addFiles(d.Platforms.Linux)
				}
			}
		}
		if extraOn {
			for _, e := range g.Extras {
				if sz, err := parseSizeString(e.Size); err == nil {
					expected += sz
				}
			}
		}
		if dlcOn {
			for _, dlc := range g.DLCs {
				for _, d := range dlc.ParsedDownloads {
					if stringsEqualFold(d.Language, lang) {
						switch stringsLower(plat) {
						case "all":
							addFiles(d.Platforms.Windows)
							addFiles(d.Platforms.Mac)
							addFiles(d.Platforms.Linux)
						case "windows":
							addFiles(d.Platforms.Windows)
						case "mac":
							addFiles(d.Platforms.Mac)
						case "linux":
							addFiles(d.Platforms.Linux)
						}
					}
				}
				if extraOn {
					for _, e := range dlc.Extras {
						if sz, err := parseSizeString(e.Size); err == nil {
							expected += sz
						}
					}
				}
			}
		}

		actual, err := g.EstimateStorageSize(lang, plat, extraOn, dlcOn)
		if err != nil {
			t.Fatalf("EstimateStorageSize returned error: %v", err)
		}
		if actual != expected {
			t.Fatalf("size mismatch: got=%d expected=%d (lang=%s plat=%s extras=%v dlc=%v)\njson=%s", actual, expected, lang, plat, extraOn, dlcOn, string(b))
		}
	}
}

// helpers to avoid importing strings just for EqualFold and ToLower if not already present
func stringsEqualFold(a, b string) bool { return stringsLower(a) == stringsLower(b) }
func stringsLower(s string) string {
	// inline to avoid extra imports if the file already had strings; but we can use standard lib
	// using standard strings package for correctness
	return strings.ToLower(s)
}
