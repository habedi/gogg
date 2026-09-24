package client

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var installerVersionPattern = regexp.MustCompile(`^(?P<prefix>.*?)(?P<ver>\d+(?:\.\d+)+)(?P<suffix>\.[^.]+)$`)

// parseInstallerVersion splits an installer's file name into what surrounds
// the version and the version itself.
func parseInstallerVersion(filename string) (prefix string, version []int, suffix string, ok bool) {
	m := installerVersionPattern.FindStringSubmatch(filename)
	if m == nil {
		return "", nil, "", false
	}
	prefix = m[1]
	suffix = m[3]
	for _, part := range strings.Split(m[2], ".") {
		value, err := strconv.Atoi(part)
		if err != nil {
			return "", nil, "", false
		}
		version = append(version, value)
	}
	return prefix, version, suffix, true
}

// compareInstallerVersions orders two versions: 1 when a is newer, -1 when b
// is, 0 when they are the same.
func compareInstallerVersions(a, b []int) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		va, vb := 0, 0
		if i < len(a) {
			va = a[i]
		}
		if i < len(b) {
			vb = b[i]
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

// installerGroup identifies files that are versions of the same installer.
// Files only compete with one another when they sit in the same directory and
// share both a name prefix and an extension: a Windows installer is not an
// older version of the Linux one, and neither is a DLC installer that happens
// to share a name with the base game.
type installerGroup struct {
	dir    string
	prefix string
	suffix string
}

// installerExtensions are the kinds of files that count as installers; nothing
// else is ever touched.
var installerExtensions = map[string]struct{}{
	".exe": {}, ".bin": {}, ".dmg": {}, ".sh": {}, ".zip": {}, ".tar.gz": {}, ".rar": {},
}

// PruneOldInstallerVersions deletes older versions of installers a download
// has just replaced, and reports what it removed: deleting files behind
// someone's back is how trust in a downloader ends. Roots that do not exist
// are simply skipped, and files it cannot remove are reported together rather
// than stopping the sweep.
func PruneOldInstallerVersions(rootPath, title string, options DownloadOptions) (removed []string, err error) {
	var roots []string
	switch {
	case options.LutrisLayout:
		roots = []string{filepath.Join(rootPath, LutrisSlug(title), "gog")}
	case options.RomMLayout:
		platforms := []string{"win", "mac", "linux"}
		if plat := RomMPlatform(options.Platform); plat != "all" && plat != "" {
			platforms = []string{plat}
		}
		for _, platform := range platforms {
			roots = append(roots, filepath.Join(rootPath, platform, SanitizePath(title)))
		}
	default:
		roots = []string{filepath.Join(rootPath, SanitizePath(title))}
	}

	for _, root := range roots {
		if info, statErr := os.Stat(root); statErr != nil || !info.IsDir() {
			continue
		}

		filesByGroup := map[installerGroup][]string{}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			name := info.Name()
			ext := filepath.Ext(name)
			if strings.HasSuffix(name, ".tar.gz") {
				ext = ".tar.gz"
			}
			if _, ok := installerExtensions[ext]; !ok {
				return nil
			}
			prefix, _, suffix, ok := parseInstallerVersion(name)
			if !ok {
				return nil
			}
			group := installerGroup{dir: filepath.Dir(path), prefix: prefix, suffix: suffix}
			filesByGroup[group] = append(filesByGroup[group], path)
			return nil
		})

		for _, paths := range filesByGroup {
			var best string
			var bestVersion []int
			for _, path := range paths {
				_, version, _, ok := parseInstallerVersion(filepath.Base(path))
				if !ok {
					continue
				}
				if best == "" || compareInstallerVersions(version, bestVersion) == 1 {
					best = path
					bestVersion = version
				}
			}
			for _, path := range paths {
				if path == best {
					continue
				}
				if rmErr := os.Remove(path); rmErr != nil {
					err = errors.Join(err, rmErr)
					continue
				}
				removed = append(removed, path)
			}
		}
	}
	return removed, err
}
