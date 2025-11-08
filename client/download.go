package client

import (
	"context"
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

// progressReader wraps an io.Reader to send progress updates through an io.Writer.
type progressReader struct {
	reader     io.Reader
	writer     io.Writer
	fileName   string
	totalSize  int64
	bytesRead  int64
	updateLock sync.Mutex
}

func (pr *progressReader) Write(p []byte) (int, error) {
	pr.updateLock.Lock()
	defer pr.updateLock.Unlock()

	n, err := pr.writer.Write(p)
	if err != nil {
		log.Error().Err(err).Msg("Failed to write progress update")
	}
	return n, err
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		pr.updateLock.Lock()
		pr.bytesRead += int64(n)
		currentBytes := pr.bytesRead
		pr.updateLock.Unlock()

		update := ProgressUpdate{
			Type:         "file_progress",
			FileName:     pr.fileName,
			CurrentBytes: currentBytes,
			TotalBytes:   pr.totalSize,
		}
		jsonUpdate, _ := json.Marshal(update)
		_, _ = pr.Write(append(jsonUpdate, '\n'))
	}
	return n, err
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
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			log.Error().Msgf("Path %s exists but is not a directory", path)
			return fmt.Errorf("path %s exists but is not a directory", path)
		}
		return nil
	}
	if os.IsNotExist(err) {
		log.Info().Msgf("Creating directory: %s", path)
		return os.MkdirAll(path, 0755)
	}
	log.Error().Err(err).Msgf("Error checking directory %s", path)
	return err
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
	return name
}

type downloadTask struct {
	url      string
	fileName string
	subDir   string
	resume   bool
	flatten  bool
}

func DownloadGameFiles(
	ctx context.Context,
	accessToken string, game Game, downloadPath string,
	gameLanguage string, platformName string, extrasFlag bool, dlcFlag bool, resumeFlag bool,
	flattenFlag bool, skipPatchesFlag bool, numThreads int,
	updateWriter io.Writer,
) error {
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

	totalDownloadSize, err := game.EstimateStorageSize(gameLanguage, platformName, extrasFlag, dlcFlag)
	if err != nil {
		return fmt.Errorf("failed to estimate total download size: %w", err)
	}
	startUpdate := ProgressUpdate{Type: "start", OverallTotalBytes: totalDownloadSize}
	jsonStart, _ := json.Marshal(startUpdate)
	_, _ = fmt.Fprintln(sw, string(jsonStart))

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
					if fileName == "" {
						fileName = base
					}
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
		targetDir := filepath.Join(downloadPath, SanitizePath(game.Title), SanitizePath(subDir))
		filePath := filepath.Join(targetDir, fileName)

		if err := ensureDirExists(targetDir); err != nil {
			return err
		}

		var file *os.File
		var startOffset int64

		if task.resume {
			if fileInfo, statErr := os.Stat(filePath); statErr == nil {
				startOffset = fileInfo.Size()
				file, err = os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
			} else if os.IsNotExist(statErr) {
				file, err = os.Create(filePath)
			} else {
				return statErr
			}
		} else {
			file, err = os.Create(filePath)
		}
		if err != nil {
			return err
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

		totalSize := headResp.ContentLength
		if task.resume && totalSize > 0 && startOffset >= totalSize {
			// File is already complete, send a final progress update for it.
			finalUpdate := ProgressUpdate{Type: "file_progress", FileName: fileName, CurrentBytes: startOffset, TotalBytes: totalSize}
			jsonUpdate, _ := json.Marshal(finalUpdate)
			_, _ = fmt.Fprintln(sw, string(jsonUpdate))
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
			return fmt.Errorf("failed to download %s: HTTP %d", fileName, getResp.StatusCode)
		}

		// If the server ignored Range and returned 200, make sure we start from the beginning
		if requestedRange > 0 && getResp.StatusCode == http.StatusOK {
			if err := file.Close(); err != nil {
				return err
			}
			file, err = os.Create(filePath) // truncate
			if err != nil {
				return err
			}
			defer func() { _ = file.Close() }()
			startOffset = 0
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
		nWritten, err := io.CopyBuffer(file, progressReader, buffer)
		if err != nil {
			// Tolerate ErrUnexpectedEOF if we actually received the exact expected remaining bytes
			if errors.Is(err, io.ErrUnexpectedEOF) && totalSize > 0 {
				if startOffset+int64(nWritten) == totalSize {
					err = nil
				}
			}
		}
		if err != nil {
			if ctx.Err() == context.Canceled || ctx.Err() == context.DeadlineExceeded {
				// On cancellation, remove partial file unless resume was requested
				if !task.resume {
					_ = file.Close()
					_ = os.Remove(filePath)
					log.Warn().Str("file", filePath).Msg("Download cancelled, removed partial file")
				}
				return ctx.Err()
			}
			// On other errors, keep partial if resume, else remove
			if !task.resume {
				_ = file.Close()
				_ = os.Remove(filePath)
			}
			return fmt.Errorf("failed to save file %s: %w", filePath, err)
		}
		return nil
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

	downloadErrors := pool.Run(ctx, tasks, numThreads, downloadFile)

	if len(downloadErrors) > 0 {
		for _, err := range downloadErrors {
			if err != context.Canceled && err != context.DeadlineExceeded {
				log.Error().Err(err).Msg("Worker failed to download file")
			}
		}
		return fmt.Errorf("%d download tasks failed or were cancelled, first error: %w", len(downloadErrors), downloadErrors[0])
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		metadataPath := filepath.Join(downloadPath, SanitizePath(game.Title), "metadata.json")
		metadata, err := json.MarshalIndent(game, "", "  ")
		if err == nil {
			if ensureDirExists(filepath.Dir(metadataPath)) == nil {
				_ = os.WriteFile(metadataPath, metadata, 0644)
			}
		}
	}

	log.Info().Msg("Download process completed.")
	return nil
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
