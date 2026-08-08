package client

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	netURL "net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/habedi/gogg/pkg/pool"
	"github.com/rs/zerolog/log"
)

// ProgressUpdate defines the structure for progress messages.
type ProgressUpdate struct {
	Type              string `json:"type"` // "start", "file_progress", "status"
	FileName          string `json:"file,omitempty"`
	CurrentBytes      int64  `json:"current,omitempty"`
	TotalBytes        int64  `json:"total,omitempty"`
	OverallTotalBytes int64  `json:"overall_total,omitempty"`
}

// syncWriter serializes writes to an underlying writer.
type syncWriter struct {
	w  io.Writer
	mu *sync.Mutex
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// progressInterval is how often a transfer reports its progress. Reporting
// every read would mean over a million updates for a large game, each one
// marshalled, serialised through a mutex and decoded by the reader.
var progressInterval = 200 * time.Millisecond

// progressReader wraps an io.Reader to send progress updates through an io.Writer.
type progressReader struct {
	reader     io.Reader
	writer     io.Writer
	fileName   string
	totalSize  int64
	bytesRead  int64
	reported   int64
	lastReport time.Time
	updateLock sync.Mutex
}

func (pr *progressReader) writeProgress(data []byte) {
	if _, err := pr.writer.Write(data); err != nil {
		log.Error().Err(err).Msg("Failed to write progress update")
	}
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.updateLock.Lock()
		pr.bytesRead += int64(n)
		pr.updateLock.Unlock()
	}

	// The end of the transfer is always reported, so a throttled update can
	// never leave a file looking unfinished.
	if n > 0 || err != nil {
		pr.report(err != nil)
	}
	return n, err
}

// report sends the current progress unless it was sent too recently. A final
// report is always sent, provided there is something new to say.
func (pr *progressReader) report(final bool) {
	pr.updateLock.Lock()
	now := time.Now()
	switch {
	case final:
		if pr.reported == pr.bytesRead {
			pr.updateLock.Unlock()
			return
		}
	case !pr.lastReport.IsZero() && now.Sub(pr.lastReport) < progressInterval:
		pr.updateLock.Unlock()
		return
	}
	pr.lastReport = now
	pr.reported = pr.bytesRead
	currentBytes := pr.bytesRead
	pr.updateLock.Unlock()

	update := ProgressUpdate{
		Type:         "file_progress",
		FileName:     pr.fileName,
		CurrentBytes: currentBytes,
		TotalBytes:   pr.totalSize,
	}
	jsonUpdate, jsonErr := json.Marshal(update)
	if jsonErr != nil {
		log.Error().Err(jsonErr).Msg("Failed to marshal progress update")
		return
	}
	pr.writeProgress(append(jsonUpdate, '\n'))
}

func ParseGameData(data string) (Game, error) {
	var rawResponse Game
	if err := json.Unmarshal([]byte(data), &rawResponse); err != nil {
		log.Error().Err(err).Msg("Failed to parse game data")
		return Game{}, err
	}
	return rawResponse, nil
}

func ensureDirExists(path string) error {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to create directory: %s", path)
		return err
	}

	info, err := os.Stat(path)
	if err != nil {
		log.Error().Err(err).Msgf("Error checking directory %s", path)
		return err
	}
	if !info.IsDir() {
		log.Error().Msgf("Path %s exists but is not a directory", path)
		return fmt.Errorf("path %s exists but is not a directory", path)
	}
	return nil
}

var pathSanitizer = strings.NewReplacer(
	"®", "",
	"™", "",
	":", "",
	"/", "-",
	"\\", "-",
	"*", "-",
	"?", "-",
	"<", "-",
	">", "-",
	"|", "-",
	"\"", "",
	"'", "",
	" ", "-",
	"(", "",
	")", "",
)

var multiDash = regexp.MustCompile(`-+`)
var allowedChars = regexp.MustCompile(`[^a-z0-9._-]+`)

func SanitizePath(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = pathSanitizer.Replace(name)
	name = allowedChars.ReplaceAllString(name, "")
	name = multiDash.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")

	// If empty, return empty to preserve previous behavior
	if name == "" {
		return ""
	}

	const maxPathLength = 200
	if len(name) > maxPathLength {
		name = strings.TrimRight(name[:maxPathLength], "-")
	}
	return name
}

type downloadTask struct {
	url      string
	fileName string
	subDir   string
	resume   bool
	flatten  bool
}

// DownloadOptions says what DownloadGameFiles should fetch and how it should
// land on disk. The zero value downloads nothing useful: Language, Platform,
// and Threads have no defaults worth guessing.
type DownloadOptions struct {
	// Language is the full name the game data spells it in, such as "English".
	Language string
	// Platform is windows, mac, linux, or all.
	Platform string
	// Extras and DLCs say whether what surrounds the game comes too.
	Extras bool
	DLCs   bool
	// Resume picks partially downloaded files up where they stopped.
	Resume bool
	// Flatten puts every file in one folder instead of GOG's layout.
	Flatten bool
	// SkipPatches leaves patch files out.
	SkipPatches bool
	// RomMLayout arranges folders as platform/game.
	RomMLayout bool
	// LutrisLayout arranges folders as <lutris-slug>/gog, the way Lutris
	// caches installer files, so Lutris finds them instead of re-downloading.
	// It flattens: every file of the game lands in that one folder.
	LutrisLayout bool
	// Threads is how many files are transferred at once.
	Threads int
}

func DownloadGameFiles(
	ctx context.Context,
	accessToken string, game Game, downloadPath string,
	options DownloadOptions,
	updateWriter io.Writer,
) error {
	gameLanguage, platformName := options.Language, options.Platform
	extrasFlag, dlcFlag, resumeFlag := options.Extras, options.DLCs, options.Resume
	flattenFlag, skipPatchesFlag, rommLayout := options.Flatten, options.SkipPatches, options.RomMLayout
	lutrisLayout := options.LutrisLayout
	numThreads := options.Threads

	if rommLayout && lutrisLayout {
		return fmt.Errorf("the RomM and Lutris layouts cannot both be used")
	}

	// This transport is configured for large file downloads. It has connection
	// timeouts but no total timeout, preventing failures on slow networks.
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	client := &http.Client{Transport: transport}
	clientNoRedirect := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	if err := ensureDirExists(downloadPath); err != nil {
		log.Error().Err(err).Msgf("Failed to create download path %s", downloadPath)
		return err
	}

	// Serialize all progress JSON output
	sw := &syncWriter{w: updateWriter, mu: &sync.Mutex{}}

	// The metadata and the file manifest belong with the files they describe.
	// Under the RomM layout those live in <root>/<platform>/<game>; with "all"
	// they are spread across platforms, so it stays at the top level.
	manifestDir := filepath.Join(downloadPath, SanitizePath(game.Title))
	if lutrisLayout {
		manifestDir = filepath.Join(downloadPath, LutrisSlug(game.Title), "gog")
	} else if rommLayout {
		if plat := strings.ToLower(strings.TrimSpace(platformName)); plat != "" && plat != "all" {
			manifestDir = filepath.Join(downloadPath, plat, SanitizePath(game.Title))
		}
	}

	// What this run brought, for the manifest written at the end.
	var manifestMu sync.Mutex
	var broughtFiles []DownloadedFile
	recordFile := func(entry DownloadedFile) {
		manifestMu.Lock()
		defer manifestMu.Unlock()
		broughtFiles = append(broughtFiles, entry)
	}

	totalDownloadSize, err := game.EstimateStorageSize(gameLanguage, platformName, extrasFlag, dlcFlag)
	if err != nil {
		return fmt.Errorf("failed to estimate total download size: %w", err)
	}
	startUpdate := ProgressUpdate{Type: "start", OverallTotalBytes: totalDownloadSize}
	jsonStart, jsonErr := json.Marshal(startUpdate)
	if jsonErr != nil {
		log.Error().Err(jsonErr).Msg("Failed to marshal start update")
	} else {
		_, _ = fmt.Fprintln(sw, string(jsonStart))
	}

	findFileLocation := func(ctx context.Context, url string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		resp, err := clientNoRedirect.Do(req)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return "", ctx.Err()
			}
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			if location := resp.Header.Get("Location"); location != "" {
				return location, nil
			}
			return "", fmt.Errorf("redirect location not found in header")
		}
		return "", nil
	}

	downloadFile := func(ctx context.Context, task downloadTask) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		url := task.url
		fileName := task.fileName

		location, err := findFileLocation(ctx, url)
		if err != nil {
			return fmt.Errorf("failed redirect check for %s: %w", url, err)
		}
		if location != "" {
			url = location
			if parsedLoc, parseErr := netURL.Parse(location); parseErr == nil && parsedLoc.Path != "" {
				if base := filepath.Base(parsedLoc.Path); base != "." && base != "/" {
					fileName = base
				}
			}
		}

		if decodedFileName, err := netURL.QueryUnescape(fileName); err == nil {
			fileName = decodedFileName
		}
		// Strip any query remnants from filename (safety)
		if q := strings.IndexByte(fileName, '?'); q >= 0 {
			fileName = fileName[:q]
		}

		subDir := task.subDir
		if task.flatten {
			subDir = ""
		}
		var targetDir string
		switch {
		case lutrisLayout:
			// Lutris cache layout: <slug>/gog/, flat, however the file is
			// grouped by GOG.
			targetDir = filepath.Join(downloadPath, LutrisSlug(game.Title), "gog")
		case rommLayout:
			// RomM layout: platform/game/
			plat := strings.ToLower(strings.TrimSpace(strings.Split(subDir, string(os.PathSeparator))[0]))
			if plat == "" {
				plat = strings.ToLower(platformName)
			}
			targetDir = filepath.Join(downloadPath, plat, SanitizePath(game.Title))
		default:
			targetDir = filepath.Join(downloadPath, SanitizePath(game.Title), SanitizePath(subDir))
		}
		filePath := filepath.Join(targetDir, fileName)

		if err := ensureDirExists(targetDir); err != nil {
			return err
		}

		var file *os.File
		var startOffset int64
		partPath := filePath + ".part"
		usingPartFile := false

		if task.resume {
			if fileInfo, statErr := os.Stat(partPath); statErr == nil {
				// Resume an in-progress download.
				startOffset = fileInfo.Size()
				file, err = os.OpenFile(partPath, os.O_APPEND|os.O_WRONLY, 0644)
				if err != nil {
					return err
				}
				usingPartFile = true
			} else {
				if fileInfo, statErr := os.Stat(filePath); statErr == nil {
					// Final file already present (completed or legacy partial).
					startOffset = fileInfo.Size()
					file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
					if err != nil {
						return err
					}
				} else if os.IsNotExist(statErr) {
					file, err = os.Create(partPath)
					if err != nil {
						return err
					}
					usingPartFile = true
				} else {
					return statErr
				}
			}
		} else {
			file, err = os.Create(partPath)
			if err != nil {
				return err
			}
			usingPartFile = true
		}
		defer func() { _ = file.Close() }()

		headReq, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
		if err != nil {
			return err
		}
		headReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))

		headResp, err := client.Do(headReq)
		if err != nil {
			return err
		}
		_ = headResp.Body.Close()
		if headResp.StatusCode == http.StatusForbidden {
			log.Warn().Str("file", fileName).Str("url", url).Msg("Skipping file: server returned HTTP 403; file may be bundled in the main installer")
			_ = file.Close()
			if usingPartFile {
				_ = os.Remove(partPath)
			} else if !task.resume {
				_ = os.Remove(filePath)
			}
			return nil
		}

		totalSize := headResp.ContentLength
		if task.resume && totalSize > 0 && startOffset >= totalSize {
			// File is already complete, send a final progress update for it.
			finalUpdate := ProgressUpdate{Type: "file_progress", FileName: fileName, CurrentBytes: startOffset, TotalBytes: totalSize}
			jsonUpdate, _ := json.Marshal(finalUpdate)
			_, _ = fmt.Fprintln(sw, string(jsonUpdate))
			if usingPartFile {
				_ = file.Close()
				_ = os.Rename(partPath, filePath)
			}
			return nil
		}

		getReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return err
		}
		getReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", accessToken))
		requestedRange := int64(0)
		if task.resume && startOffset > 0 {
			getReq.Header.Set("Range", fmt.Sprintf("bytes=%d-", startOffset))
			requestedRange = startOffset
		}

		getResp, err := client.Do(getReq)
		if err != nil {
			return err
		}
		defer func() { _ = getResp.Body.Close() }()

		if getResp.StatusCode != http.StatusOK && getResp.StatusCode != http.StatusPartialContent {
			if getResp.StatusCode == http.StatusForbidden {
				log.Warn().Str("file", fileName).Str("url", url).Msg("Skipping file: server returned HTTP 403; file may be bundled in the main installer")
				_ = file.Close()
				if usingPartFile {
					_ = os.Remove(partPath)
				} else if !task.resume {
					_ = os.Remove(filePath)
				}
				return nil
			}
			if !task.resume {
				_ = file.Close()
				activeFile := filePath
				if usingPartFile {
					activeFile = partPath
				}
				_ = os.Remove(activeFile)
			}
			return fmt.Errorf("failed to download %s: %w", fileName, &httpStatusError{status: getResp.StatusCode})
		}

		// If the server ignored Range and returned 200, make sure we start from the beginning
		if requestedRange > 0 && getResp.StatusCode == http.StatusOK {
			if err := file.Close(); err != nil {
				return err
			}
			truncPath := filePath
			if usingPartFile {
				truncPath = partPath
			}
			file, err = os.Create(truncPath) // truncate
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			startOffset = 0
		}
		// The checksum is computed as the bytes stream past. A resumed download
		// already holds a prefix, which is read back through the hash first; a
		// prefix that cannot be read leaves the checksum unrecorded rather than
		// recorded wrong.
		hasher := md5.New()
		if startOffset > 0 {
			activeFile := filePath
			if usingPartFile {
				activeFile = partPath
			}
			prior, hashErr := os.Open(activeFile)
			if hashErr == nil {
				_, hashErr = io.Copy(hasher, io.LimitReader(prior, startOffset))
				_ = prior.Close()
			}
			if hashErr != nil {
				hasher = nil
			}
		}
		var sink io.Writer = file
		if hasher != nil {
			sink = io.MultiWriter(file, hasher)
		}

		limitedBody := wrapWithGlobalRateLimiter(getResp.Body)
		progressReader := &progressReader{
			reader:    limitedBody,
			writer:    sw,
			fileName:  fileName,
			totalSize: totalSize,
			bytesRead: startOffset,
		}

		buffer := make([]byte, 32*1024)
		nWritten, err := io.CopyBuffer(sink, progressReader, buffer)
		if err != nil {
			// Tolerate ErrUnexpectedEOF if we actually received the exact expected remaining bytes
			if errors.Is(err, io.ErrUnexpectedEOF) && totalSize > 0 {
				if startOffset+int64(nWritten) == totalSize {
					err = nil
				}
			}
		}
		if err != nil {
			activeFile := filePath
			if usingPartFile {
				activeFile = partPath
			}
			if isCancellation(ctx.Err()) {
				// On cancellation, remove partial file unless resume was requested
				if !task.resume {
					_ = file.Close()
					_ = os.Remove(activeFile)
					log.Warn().Str("file", activeFile).Msg("Download cancelled, removed partial file")
				}
				return ctx.Err()
			}
			// On other errors, keep partial if resume, else remove
			if !task.resume {
				_ = file.Close()
				_ = os.Remove(activeFile)
			}
			return fmt.Errorf("failed to save file %s: %w", filePath, err)
		}
		if usingPartFile {
			_ = file.Close()
			if err := os.Rename(partPath, filePath); err != nil {
				return fmt.Errorf("failed to finalize %s: %w", fileName, err)
			}
		}

		sum := ""
		if hasher != nil {
			sum = hex.EncodeToString(hasher.Sum(nil))
		}
		relPath := filePath
		if rel, relErr := filepath.Rel(manifestDir, filePath); relErr == nil && !strings.HasPrefix(rel, "..") {
			relPath = rel
		}
		recordFile(DownloadedFile{
			Path:         relPath,
			SizeBytes:    startOffset + nWritten,
			MD5:          sum,
			DownloadedAt: time.Now().UTC(),
		})
		return nil
	}

	// Transient failures are retried with a pause: a download hours in is not
	// abandoned over one dropped connection. With resume on, an attempt picks
	// up where the last one stopped.
	downloadWithRetry := func(ctx context.Context, task downloadTask) error {
		var err error
		for attempt := 1; attempt <= downloadAttempts; attempt++ {
			if attempt > 1 {
				log.Warn().Str("file", task.fileName).Int("attempt", attempt).Err(err).
					Msg("Retrying download after a transient failure")
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(RetryDelay << (attempt - 2)):
				}
			}
			err = downloadFile(ctx, task)
			if err == nil || isCancellation(err) || !isRetryable(err) {
				return err
			}
		}
		return err
	}

	var tasks []downloadTask
	var tasksMutex sync.Mutex

	enqueue := func(t downloadTask) {
		tasksMutex.Lock()
		defer tasksMutex.Unlock()
		tasks = append(tasks, t)
	}

	var enqueueErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		enqueueErr = func() error {
			if err := enqueueGameFiles(ctx, enqueue, game, gameLanguage, platformName, "", resumeFlag, flattenFlag, skipPatchesFlag); err != nil {
				return err
			}
			if extrasFlag {
				if err := enqueueExtras(ctx, enqueue, game.Extras, "extras", resumeFlag, flattenFlag); err != nil {
					return err
				}
			}
			if dlcFlag {
				if err := enqueueDLCs(ctx, enqueue, &game, gameLanguage, platformName, extrasFlag, resumeFlag, flattenFlag, skipPatchesFlag); err != nil {
					return err
				}
			}
			return nil
		}()
	}()
	wg.Wait()

	if enqueueErr != nil {
		return enqueueErr
	}

	downloadErrors := pool.Run(ctx, tasks, numThreads, downloadWithRetry)

	if len(downloadErrors) > 0 {
		for _, err := range downloadErrors {
			if !isCancellation(err) {
				log.Error().Err(err).Msg("Worker failed to download file")
			}
		}
		return fmt.Errorf("%d download tasks failed or were cancelled, first error: %w", len(downloadErrors), downloadErrors[0])
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		metadataPath := filepath.Join(manifestDir, "metadata.json")
		metadata, err := json.MarshalIndent(game, "", "  ")
		if err == nil {
			if ensureDirExists(filepath.Dir(metadataPath)) == nil {
				_ = os.WriteFile(metadataPath, metadata, 0644)
			}
		}
		manifestMu.Lock()
		brought := append([]DownloadedFile{}, broughtFiles...)
		manifestMu.Unlock()
		writeFileManifest(filepath.Join(manifestDir, "files.json"), brought)
	}

	log.Info().Msg("Download process completed.")
	return nil
}

// DownloadedFile is one file a download brought, as files.json beside
// metadata.json records it: where it landed, exactly how many bytes it is,
// the MD5 of what streamed in, and when. The checksum is of what was written,
// so a file can later be told apart from what it was.
type DownloadedFile struct {
	Path         string    `json:"path"` // relative to the game's folder when under it
	SizeBytes    int64     `json:"size_bytes"`
	MD5          string    `json:"md5,omitempty"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

// writeFileManifest records what this run downloaded, keeping the entries of
// files earlier runs brought and this one skipped.
func writeFileManifest(path string, fresh []DownloadedFile) {
	entries := make(map[string]DownloadedFile)
	if data, err := os.ReadFile(path); err == nil {
		var existing []DownloadedFile
		if json.Unmarshal(data, &existing) == nil {
			for _, entry := range existing {
				entries[entry.Path] = entry
			}
		}
	}
	for _, entry := range fresh {
		entries[entry.Path] = entry
	}

	all := make([]DownloadedFile, 0, len(entries))
	for _, entry := range entries {
		all = append(all, entry)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Path < all[j].Path })

	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	if ensureDirExists(filepath.Dir(path)) != nil {
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Debug().Err(err).Msg("Could not write the file manifest")
	}
}

// isCancellation reports whether err is, or wraps, a context cancellation or a
// deadline. Download errors are wrapped before they get here, so a direct
// comparison against the sentinel values would never match.
func isCancellation(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// downloadAttempts is how many times one file is tried before its download
// counts as failed; RetryDelay is the pause before the second try, doubling
// after. A variable so tests, including other packages' tests that download
// through here, do not sit the delays out.
const downloadAttempts = 3

var RetryDelay = 2 * time.Second

// httpStatusError is a download refusal with the status the server gave, kept
// as a type so retrying can tell a server mistake from a server refusal.
type httpStatusError struct {
	status int
}

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.status) }

// isRetryable reports whether trying again can help: network trouble and
// server-side errors can pass, a refusal like 404 will not.
func isRetryable(err error) bool {
	var status *httpStatusError
	if errors.As(err, &status) {
		return status.status >= 500 ||
			status.status == http.StatusTooManyRequests ||
			status.status == http.StatusRequestTimeout
	}
	// Everything else that is not a cancellation is assumed transient:
	// dropped connections and read timeouts do not come typed.
	return true
}

func isAbsoluteURL(u string) bool {
	parsed, err := netURL.Parse(u)
	return err == nil && parsed.Scheme != "" && parsed.Host != ""
}

func buildManualURL(u string) string {
	if isAbsoluteURL(u) {
		return u
	}
	return fmt.Sprintf("https://embed.gog.com%s", u)
}

func enqueueGameFiles(ctx context.Context, enqueue func(downloadTask), game Game, lang, platform, subDirPrefix string, resume, flatten, skipPatches bool) error {
	for _, download := range game.Downloads {
		if !strings.EqualFold(download.Language, lang) {
			continue
		}
		platforms := map[string][]PlatformFile{
			"windows": download.Platforms.Windows, "mac": download.Platforms.Mac, "linux": download.Platforms.Linux,
		}
		for name, files := range platforms {
			if platform != "all" && !strings.EqualFold(platform, name) {
				continue
			}
			for _, file := range files {
				if file.ManualURL == nil || *file.ManualURL == "" {
					continue
				}
				if skipPatches && (strings.Contains(strings.ToLower(*file.ManualURL), "patch") || strings.Contains(strings.ToLower(file.Name), "patch")) {
					continue
				}
				task := downloadTask{
					url:      buildManualURL(*file.ManualURL),
					fileName: file.Name,
					subDir:   filepath.Join(subDirPrefix, name),
					resume:   resume,
					flatten:  flatten,
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					enqueue(task)
				}
			}
		}
	}
	return nil
}

func enqueueExtras(ctx context.Context, enqueue func(downloadTask), extras []Extra, subDir string, resume, flatten bool) error {
	for _, extra := range extras {
		if extra.ManualURL == "" {
			continue
		}
		fileName := SanitizePath(extra.Name)
		if ext := filepath.Ext(extra.ManualURL); ext != "" {
			fileName += ext
		}
		task := downloadTask{
			url:      buildManualURL(extra.ManualURL),
			fileName: fileName,
			subDir:   subDir,
			resume:   resume,
			flatten:  flatten,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			enqueue(task)
		}
	}
	return nil
}

func enqueueDLCs(ctx context.Context, enqueue func(downloadTask), game *Game, lang, platform string, extras, resume, flatten, skipPatches bool) error {
	for _, dlc := range game.DLCs {
		dlcSubDir := filepath.Join("dlcs", SanitizePath(dlc.Title))
		dlcGame := Game{Title: dlc.Title, Downloads: dlc.ParsedDownloads}
		if err := enqueueGameFiles(ctx, enqueue, dlcGame, lang, platform, dlcSubDir, resume, flatten, skipPatches); err != nil {
			return err
		}
		if extras {
			if err := enqueueExtras(ctx, enqueue, dlc.Extras, filepath.Join(dlcSubDir, "extras"), resume, flatten); err != nil {
				return err
			}
		}
	}
	return nil
}
