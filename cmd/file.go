package cmd

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

var hashAlgorithms = []string{"md5", "sha1", "sha256", "sha512"}

// fileCmd represents the file command
// It returns a cobra.Command that performs various file operations.
func fileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Perform various file operations",
	}

	// Add subcommands to the file command
	cmd.AddCommand(hashCmd())

	return cmd
}

// hashCmd represents the hash command
// It returns a cobra.Command that generates hash values for files in a directory.
func hashCmd() *cobra.Command {
	var saveToFileFlag bool
	var cleanFlag bool
	var algo string
	var recursiveFlag bool

	cmd := &cobra.Command{
		Use:   "hash [fileDir]",
		Short: "Generate hash values for game files in a directory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			// Validate the hash algorithm
			if !isValidHashAlgo(algo) {
				log.Error().Msgf("Unsupported hash algorithm: %s", algo)
				return
			}

			// Call the function to generate hash files
			generateHashFiles(dir, algo, recursiveFlag, saveToFileFlag, cleanFlag)
		},
	}

	// Add flags for hash options
	cmd.Flags().StringVarP(&algo, "algo", "a", "sha256", "Hash algorithm to use [md5, sha1, sha256, sha512]")
	cmd.Flags().BoolVarP(&recursiveFlag, "recursive", "r", true, "Process files in subdirectories? [true, false]")
	cmd.Flags().BoolVarP(&saveToFileFlag, "save", "s", false, "Save hash to files? [true, false]")
	cmd.Flags().BoolVarP(&cleanFlag, "clean", "c", false, "Remove old hash files before generating new ones? [true, false]")

	return cmd
}

// isValidHashAlgo checks if the specified hash algorithm is supported.
// It takes a string representing the algorithm and returns a boolean.
func isValidHashAlgo(algo string) bool {
	for _, validAlgo := range hashAlgorithms {
		if strings.ToLower(algo) == validAlgo {
			return true
		}
	}
	return false
}

// removeHashFiles removes old hash files from the directory.
// It takes a string representing the directory and a boolean indicating whether to process subdirectories.
func removeHashFiles(dir string, recursive bool) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error().Msgf("Error accessing path %q: %v", path, err)
			return err
		}

		// Skip directories if not recursive
		if info.IsDir() && !recursive {
			return filepath.SkipDir
		}

		// Remove hash files of all supported algorithms
		for _, algo := range hashAlgorithms {
			if strings.HasSuffix(path, "."+algo) {
				if err := os.Remove(path); err != nil {
					log.Error().Msgf("Error removing hash file %s: %v", path, err)
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Error().Msgf("Error removing hash files: %v", err)
	} else {
		log.Info().Msgf("Removed old hash files from %s", dir)
	}
}

// generateHashFiles generates hash files for files in a directory using the specified hash algorithm.
// It takes a string representing the directory, a string representing the algorithm, a boolean indicating whether to process subdirectories,
// a boolean indicating whether to save the hash to files, and a boolean indicating whether to remove old hash files before generating new ones.
func generateHashFiles(dir string, algo string, recursive bool, saveToFile bool, clean bool) {
	exclusionList := []string{".git", ".gitignore", ".DS_Store", "Thumbs.db",
		"desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm",
		"*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg"}

	var hashFiles []string
	var wg sync.WaitGroup
	fileChan := make(chan string)

	// Determine the number of workers
	numWorkers := runtime.NumCPU() - 2
	if numWorkers < 2 {
		numWorkers = 2
	}

	// Remove old hash files if clean flag is set
	if clean {
		log.Info().Msgf("Cleaning old hash files from %s and its subdirectories", dir)
		removeHashFiles(dir, true)
	}

	// Walk through the directory
	go func() {
		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Error().Msgf("Error accessing path %q: %v", path, err)
				return err
			}

			// Skip directories if not recursive
			if info.IsDir() {
				if path != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip excluded files
			for _, pattern := range exclusionList {
				matched, _ := filepath.Match(pattern, info.Name())
				if matched {
					return nil
				}
			}

			// Send file path to channel
			fileChan <- path
			return nil
		})
		close(fileChan)
	}()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				// Generate hash for the file
				hash, err := generateHash(path, algo)
				if err != nil {
					log.Error().Msgf("Error generating hash for file %s: %v", path, err)
					continue
				}

				if saveToFile {
					// Write the hash to a file with .algo-name extension
					hashFilePath := path + "." + algo
					err = os.WriteFile(hashFilePath, []byte(hash), 0644)
					if err != nil {
						log.Error().Msgf("Error writing hash to file %s: %v", hashFilePath, err)
						continue
					}
					hashFiles = append(hashFiles, hashFilePath)
				} else {
					// Print the hash value
					fmt.Printf("%s hash for \"%s\": %s\n", algo, path, hash)
				}
			}
		}()
	}

	wg.Wait()

	if saveToFile {
		fmt.Println("Generated hash files:")
		for _, file := range hashFiles {
			fmt.Println(file)
		}
	}
}

// generateHash generates the hash for a given file using the specified algorithm.
// It takes a string representing the file path and a string representing the algorithm, and returns the hash as a string and an error if any.
func generateHash(filePath string, algo string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var hashAlgo hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		hashAlgo = md5.New()
	case "sha1":
		hashAlgo = sha1.New()
	case "sha256":
		hashAlgo = sha256.New()
	case "sha512":
		hashAlgo = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algo)
	}

	if _, err := io.Copy(hashAlgo, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hashAlgo.Sum(nil)), nil
}
