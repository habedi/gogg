package gui

import (
	"fmt"
	"strings"

	"github.com/habedi/gogg/client"
)

func isPatchFile(f client.PlatformFile) bool {
	name := strings.ToLower(f.Name)
	if f.ManualURL != nil {
		u := strings.ToLower(*f.ManualURL)
		if strings.Contains(u, "patch") {
			return true
		}
	}
	return strings.Contains(name, "patch")
}

func buildVersionMapExtended(g client.Game, language, platform string, includeExtras, includeDLCs, includePatches bool) map[string]string {
	m := make(map[string]string)
	add := func(prefix, pName string, files []client.PlatformFile) {
		for _, f := range files {
			if !includePatches && isPatchFile(f) {
				continue
			}
			ver := ""
			if f.Version != nil {
				ver = *f.Version
			}
			key := prefix + pName + "|" + f.Name
			m[key] = ver
		}
	}
	matchLang := func(l string) bool { return strings.EqualFold(l, language) }
	includePlatform := func(p string) bool { return platform == "all" || strings.EqualFold(p, platform) }
	for _, dl := range g.Downloads {
		if !matchLang(dl.Language) {
			continue
		}
		if includePlatform("windows") {
			add("", "windows", dl.Platforms.Windows)
		}
		if includePlatform("mac") {
			add("", "mac", dl.Platforms.Mac)
		}
		if includePlatform("linux") {
			add("", "linux", dl.Platforms.Linux)
		}
	}
	if includeExtras {
		for _, e := range g.Extras {
			m["extras|"+e.Name] = ""
		}
	}
	if includeDLCs {
		for _, dlc := range g.DLCs {
			for _, dl := range dlc.ParsedDownloads {
				if !matchLang(dl.Language) {
					continue
				}
				platforms := []struct {
					name  string
					files []client.PlatformFile
				}{{"windows", dl.Platforms.Windows}, {"mac", dl.Platforms.Mac}, {"linux", dl.Platforms.Linux}}
				for _, pf := range platforms {
					if includePlatform(pf.name) {
						add("dlc:"+client.SanitizePath(dlc.Title)+"|", pf.name, pf.files)
					}
				}
			}
			if includeExtras {
				for _, e := range dlc.Extras {
					m["dlc_extras:"+client.SanitizePath(dlc.Title)+"|"+e.Name] = ""
				}
			}
		}
	}
	return m
}

// describeAddedFile says a file was not there the last time the game was
// fetched.
func describeAddedFile(key, version string) string {
	if version == "" {
		return fileInWords(key) + ": new"
	}
	return fmt.Sprintf("%s: new, version %s", fileInWords(key), version)
}

// describeChangedFile says a file has moved on since it was fetched.
func describeChangedFile(key, from, to string) string {
	switch {
	case from == "":
		return fmt.Sprintf("%s: now version %s", fileInWords(key), to)
	case to == "":
		return fmt.Sprintf("%s: no longer has a version", fileInWords(key))
	default:
		return fmt.Sprintf("%s: %s → %s", fileInWords(key), from, to)
	}
}

// fileInWords turns the key the version map uses into the file it stands for.
// The key is a path through the platform, the DLC and the extras a file belongs
// to, which is how gogg tells two files apart rather than something to read.
func fileInWords(key string) string {
	parts := strings.Split(key, "|")
	var about []string
	switch {
	case strings.HasPrefix(parts[0], "dlc:"):
		about = append(about, strings.TrimPrefix(parts[0], "dlc:"))
		parts = parts[1:]
	case strings.HasPrefix(parts[0], "dlc_extras:"):
		about = append(about, strings.TrimPrefix(parts[0], "dlc_extras:"), "extra")
		parts = parts[1:]
	case parts[0] == "extras":
		about = append(about, "extra")
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return key
	}

	name := parts[len(parts)-1]
	// What is left in front of the name is the platform, which extras have none
	// of.
	if len(parts) > 1 {
		about = append([]string{platformInWords(parts[0])}, about...)
	}
	if len(about) == 0 {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, strings.Join(about, ", "))
}
