package client

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var numericVersionPattern = regexp.MustCompile(`^\d+(?:\.\d+)+$`)

var installerVersionPattern = regexp.MustCompile(`^(?P<prefix>.*?)(?P<ver>\d+(?:\.\d+)+)(?P<suffix>\.tar\.gz|\.[^.]+)$`)

// GOG Windows installers may include an architecture, a build ID, and a part number.
var gogBuildPattern = regexp.MustCompile(`^(setup_.+_)\d+(?:\.\d+)*(_\((?:32|64)bit\))?_\((\d+)\)((?:-\d+)?\.(?:bin|exe))$`)

// Linux and Mac packages can end in a version and a separate GOG build number.
var packageBuildPattern = regexp.MustCompile(`^(.+_)\d+(?:\.\d+)+_(?:\((\d+)\)|(\d+))(\.(?:sh|pkg|dmg|tar\.gz))$`)

// parseInstallerVersion splits an installer's file name into what surrounds
// the version and the version itself.
func parseInstallerVersion(filename string) (prefix string, version []int, suffix string, ok bool) {
	if m := gogBuildPattern.FindStringSubmatch(filename); m != nil {
		build, err := strconv.Atoi(m[3])
		if err != nil {
			return "", nil, "", false
		}
		return m[1], []int{build}, m[2] + "_()" + m[4], true
	}
	if m := packageBuildPattern.FindStringSubmatch(filename); m != nil {
		build, err := strconv.Atoi(m[2] + m[3])
		return m[1], []int{build}, "_build" + m[4], err == nil
	}
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

// InstallerInfo identifies the catalogue selection that supplied a file.
type InstallerInfo struct {
	GameID   int    `json:"game_id,omitempty"`
	Product  string `json:"product"`
	DLC      bool   `json:"dlc,omitempty"`
	Platform string `json:"platform"`
	Language string `json:"language"`
	Version  string `json:"version,omitempty"`
}

// InstallerCleanup holds the completed passes of one download request.
type InstallerCleanup struct {
	passes     []installerPass
	incomplete bool
}

type installerPass struct {
	dir   string
	files []DownloadedFile
}

var fileManifestMu sync.Mutex

func readFileManifest(path string) ([]DownloadedFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []DownloadedFile
	err = json.Unmarshal(data, &entries)
	return entries, err
}

func saveFileManifest(path string, entries []DownloadedFile) error {
	if err := ensureDirExists(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".files-*.tmp")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	err = enc.Encode(entries)
	if err = errors.Join(err, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// PruneOldInstallerVersions removes recorded files superseded by a completed download.
func PruneOldInstallerVersions(rootPath, title string, options DownloadOptions) ([]string, error) {
	if dir := SanitizePath(title); !filepath.IsLocal(dir) || dir == "." {
		return nil, fmt.Errorf("invalid installer directory for %q", title)
	}
	if options.Cleanup == nil || options.Cleanup.incomplete {
		return nil, nil
	}
	fileManifestMu.Lock()
	defer fileManifestMu.Unlock()
	var removed []string
	for _, pass := range options.Cleanup.passes {
		if len(pass.files) == 0 {
			continue
		}
		paths, err := pruneInstallerPass(rootPath, title, options, pass)
		removed = append(removed, paths...)
		if err != nil {
			return removed, err
		}
	}
	return removed, nil
}

func pruneInstallerPass(rootPath, title string, options DownloadOptions, pass installerPass) ([]string, error) {
	manifest := filepath.Join(pass.dir, "files.json")
	entries, err := readFileManifest(manifest)
	if err != nil {
		return nil, err
	}
	current := make(map[string]DownloadedFile, len(pass.files))
	for _, file := range pass.files {
		if _, duplicate := current[file.Path]; duplicate || file.uncertain {
			return nil, nil
		}
		current[file.Path] = file
	}
	var candidates []DownloadedFile
	for _, old := range entries {
		if _, selected := current[old.Path]; selected {
			continue
		}
		for _, fresh := range pass.files {
			if supersedes(fresh, old) {
				candidates = append(candidates, old)
				break
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	localPath := func(entry DownloadedFile) string {
		path := entry.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(pass.dir, path)
		}
		dir := filepath.Join(rootPath, SanitizePath(title))
		if options.LutrisLayout {
			dir = filepath.Join(rootPath, LutrisSlug(title), "gog")
		}
		if options.RomMLayout && entry.Installer != nil {
			dir = filepath.Join(rootPath, RomMPlatform(entry.Installer.Platform), SanitizePath(title))
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return ""
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return ""
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil || !filepath.IsLocal(rel) {
			return ""
		}
		base, err := filepath.Abs(rootPath)
		if err != nil {
			return ""
		}
		rel, err = filepath.Rel(base, path)
		if err != nil || !filepath.IsLocal(rel) {
			return ""
		}
		return rel
	}
	for _, entry := range entries {
		file, ok := current[entry.Path]
		if !ok {
			continue
		}
		if entry.SizeBytes != file.SizeBytes || (file.MD5 != "" && entry.MD5 != file.MD5) ||
			!sameInstaller(entry.Installer, file.Installer) || !matchesManifest(root, localPath(entry), entry) {
			return nil, fmt.Errorf("replacement file %q could not be confirmed", entry.Path)
		}
		delete(current, entry.Path)
	}
	if len(current) > 0 {
		return nil, fmt.Errorf("missing replacement files in %s", manifest)
	}
	gone := make(map[string]bool)
	for _, old := range candidates {
		if matchesManifest(root, localPath(old), old) {
			gone[old.Path] = true
		}
	}
	if len(gone) == 0 {
		return nil, nil
	}
	remaining := slices.DeleteFunc(slices.Clone(entries), func(e DownloadedFile) bool { return gone[e.Path] })
	if err := saveFileManifest(manifest, remaining); err != nil {
		return nil, err
	}
	var removed []string
	var removeErr error
	for _, old := range candidates {
		if !gone[old.Path] {
			continue
		}
		path := localPath(old)
		if err := root.Remove(path); err != nil {
			removeErr = errors.Join(removeErr, err)
			delete(gone, old.Path)
			continue
		}
		removed = append(removed, filepath.Join(rootPath, path))
	}
	if removeErr != nil {
		remaining = slices.DeleteFunc(entries, func(e DownloadedFile) bool { return gone[e.Path] })
		removeErr = errors.Join(removeErr, saveFileManifest(manifest, remaining))
	}
	return removed, removeErr
}

func sameSelection(a, b *InstallerInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.GameID == b.GameID && a.Product == b.Product && a.Platform == b.Platform && a.DLC == b.DLC && a.Language == b.Language
}

func sameInstaller(a, b *InstallerInfo) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func matchesManifest(root *os.Root, path string, entry DownloadedFile) bool {
	if path == "" || entry.MD5 == "" {
		return false
	}
	for part := path; part != "."; part = filepath.Dir(part) {
		info, err := root.Lstat(part)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if part == path && (!info.Mode().IsRegular() || info.Size() != entry.SizeBytes) {
			return false
		}
	}
	f, err := root.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	hash := md5.New()
	_, err = io.Copy(hash, f)
	return err == nil && hex.EncodeToString(hash.Sum(nil)) == entry.MD5
}

// Some installers spell the catalogue version with underscores. Use that value to
// tell the version apart from digits in the game title or architecture.
func installerVersion(file DownloadedFile) (string, []int, string, bool) {
	name := filepath.Base(file.Path)
	if file.Installer != nil && numericVersionPattern.MatchString(file.Installer.Version) {
		version := file.Installer.Version
		token := "_" + strings.ReplaceAll(version, ".", "_") + "_"
		if before, after, ok := strings.Cut(name, token); ok && !strings.Contains(after, token) {
			if prefix, build, suffix, ok := parseInstallerVersion(before + "_" + version + "_" + after); ok {
				return prefix, build, suffix, true
			}
		}
	}
	return parseInstallerVersion(name)
}

func supersedes(fresh, old DownloadedFile) bool {
	if old.Shared || fresh.Installer == nil || filepath.Dir(fresh.Path) != filepath.Dir(old.Path) {
		return false
	}
	info := *fresh.Installer
	if old.Installer != nil && !sameSelection(old.Installer, fresh.Installer) {
		return false
	}
	name := filepath.Base(fresh.Path)
	switch filepath.Ext(name) {
	case ".exe", ".bin", ".dmg", ".pkg", ".sh", ".zip", ".rar":
	case ".gz":
		if !strings.HasSuffix(name, ".tar.gz") {
			return false
		}
	default:
		return false
	}
	p, v, s, ok := installerVersion(fresh)
	op, ov, osuffix, ook := installerVersion(old)
	if !ok || !ook || p != op || compareInstallerVersions(v, ov) <= 0 {
		return false
	}
	if old.Installer == nil {
		// Old manifests carry no language or product. Only migrate named languages in platform folders.
		if filepath.Base(filepath.Dir(old.Path)) != info.Platform || info.DLC ||
			!slices.Contains(strings.FieldsFunc(strings.ToLower(p), func(r rune) bool { return r == '_' || r == '-' }), strings.ToLower(info.Language)) {
			return false
		}
	}
	if strings.HasSuffix(s, ".bin") && strings.Contains(s, "_()-") {
		// The launcher identifies a complete Windows installer set.
		return false
	}
	if s == osuffix {
		return true
	}
	// A new Windows installer may have fewer numbered parts.
	return strings.HasSuffix(s, "_().exe") && strings.HasPrefix(osuffix, strings.TrimSuffix(s, ".exe")+"-") && strings.HasSuffix(osuffix, ".bin")
}
