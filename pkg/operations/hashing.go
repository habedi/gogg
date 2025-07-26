package operations

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/habedi/gogg/pkg/hasher"
	"github.com/rs/zerolog/log"
)

// HashResult represents the result of a single file hashing operation.
type HashResult struct {
	File string
	Hash string
	Err  error
}

// DefaultHashExclusions is the list of patterns to exclude from hashing.
var DefaultHashExclusions = []string{
	".git", ".gitignore", ".DS_Store", "Thumbs.db", "desktop.ini",
	"*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm",
	"*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg",
}

// FindFilesToHash walks a directory and returns a slice of file paths to be processed.
func FindFilesToHash(dir string, recursive bool, exclusions []string) ([]string, error) {
	var filesToProcess []string
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if !recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		for _, pattern := range exclusions {
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				return nil
			}
		}
		filesToProcess = append(filesToProcess, path)
		return nil
	})
	return filesToProcess, walkErr
}

// GenerateHashes concurrently generates hashes for a list of files.
func GenerateHashes(ctx context.Context, files []string, algo string, numThreads int) <-chan HashResult {
	tasks := make(chan string, len(files))
	results := make(chan HashResult, len(files))

	var wg sync.WaitGroup

	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range tasks {
				select {
				case <-ctx.Done():
					return
				default:
				}

				file, err := os.Open(filePath)
				if err != nil {
					results <- HashResult{File: filePath, Err: err}
					continue
				}

				hash, err := hasher.GenerateHashFromReader(file, algo)
				file.Close() // Close the file handle
				results <- HashResult{File: filePath, Hash: hash, Err: err}
			}
		}()
	}

	for _, f := range files {
		tasks <- f
	}
	close(tasks)

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// CleanHashes walks a directory and removes files with extensions matching known hash algorithms.
func CleanHashes(dir string, recursive bool) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && !recursive && path != dir {
			return filepath.SkipDir
		}
		for _, algo := range hasher.HashAlgorithms {
			if strings.HasSuffix(info.Name(), "."+algo) {
				if err := os.Remove(path); err != nil {
					log.Warn().Err(err).Str("path", path).Msg("Failed to remove old hash file")
				}
				break
			}
		}
		return nil
	})
}
