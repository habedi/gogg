package cmd

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// List of supported hash algorithms
var hashAlgorithms = []string{"md5", "sha1", "sha256", "sha512"}

// fileCmd represents the file command
// It returns a cobra.Command that performs various file operations.
func fileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Perform various file operations",
	}

	// Add subcommands to the file command
	cmd.AddCommand(hashCmd(), sizeCmd())

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

// sizeCmd represents the size command
// It returns a cobra.Command that shows the total storage size needed to download game files.
func sizeCmd() *cobra.Command {
	var language string
	var platformName string
	var extrasFlag bool
	var dlcFlag bool
	var sizeUnit string

	cmd := &cobra.Command{
		Use:   "size [gameID]",
		Short: "Show the total storage size needed to download game files",
		Long:  "Show the total storage size needed to download game files for game with the specified ID and options in MB or GB",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			err := estimateStorageSize(args[0], strings.ToLower(language), platformName, extrasFlag, dlcFlag, sizeUnit)
			if err != nil {
				log.Fatal().Err(err).Msg("Error estimating storage size for game files to be downloaded")
			}
		},
	}

	// Add flags for size options
	cmd.Flags().StringVarP(&language, "lang", "l", "en", "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", "windows", "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", true, "Include extra content files? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", true, "Include DLC files? [true, false]")
	cmd.Flags().StringVarP(&sizeUnit, "unit", "u", "mb", "Size unit to display [mb, gb]")

	return cmd
}

// estimateStorageSize estimates the total storage size needed to download game files.
// It takes the game ID, language, platform name, flags for including extras and DLCs, and the size unit (MB or GB).
func estimateStorageSize(gameID string, language string, platformName string, extrasFlag bool, dlcFlag bool, sizeUnit string) error {

	// Check if the sizeUnit is valid
	sizeUnit = strings.ToLower(sizeUnit)
	if sizeUnit != "mb" && sizeUnit != "gb" {
		fmt.Printf("Invalid size unit: \"%s\". Unit must be mb or gb\n", sizeUnit)
		return fmt.Errorf("invalid size unit")
	}

	// Check if the language is valid
	if !isValidLanguage(language) {
		fmt.Println("Invalid language code. Supported languages are:")
		for langCode, langName := range gameLanguages {
			fmt.Printf("'%s' for %s\n", langCode, langName)
		}
		return fmt.Errorf("invalid language code")
	} else {
		language = gameLanguages[language]
	}

	// Try to convert the game ID to an integer
	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		log.Error().Msgf("Invalid game ID: %s", gameID)
		return err
	}

	// Retrieve the game data based on the game ID
	game, err := db.GetGameByID(gameIDInt)
	if err != nil {
		log.Error().Msgf("Failed to retrieve game data for ID %d: %v", gameIDInt, err)
		return err
	}

	// Check the game data is not nil
	if game == nil {
		log.Error().Msgf("Game not found for ID %d", gameIDInt)
		fmt.Printf("Game with ID %d not found in the catalogue.\n", gameIDInt)
		return err
	}

	// Unmarshal the nested JSON data
	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		log.Error().Msgf("Failed to unmarshal game data for ID %d: %v", gameIDInt, err)
		return err
	}

	// Variable to store the total size of downloads
	var totalSizeMB float64

	// Function to parse size strings
	parseSize := func(sizeStr string) (float64, error) {
		sizeStr = strings.TrimSpace(strings.ToLower(sizeStr))
		if strings.HasSuffix(sizeStr, " gb") {
			sizeStr = strings.TrimSuffix(sizeStr, " gb")
			size, err := strconv.ParseFloat(sizeStr, 64)
			if err != nil {
				return 0, err
			}
			return size * 1024, nil // Convert GB to MB
		} else if strings.HasSuffix(sizeStr, " mb") {
			sizeStr = strings.TrimSuffix(sizeStr, " mb")
			return strconv.ParseFloat(sizeStr, 64)
		}
		return 0, fmt.Errorf("unknown size unit")
	}

	// Calculate the size of downloads
	for _, download := range nestedData.Downloads {
		if strings.ToLower(download.Language) != strings.ToLower(language) {
			log.Info().Msgf("Skipping language %s", download.Language)
			continue
		}

		for _, platformFiles := range []struct {
			files  []client.PlatformFile
			subDir string
		}{
			{files: download.Platforms.Windows, subDir: "windows"},
			{files: download.Platforms.Mac, subDir: "mac"},
			{files: download.Platforms.Linux, subDir: "linux"},
		} {
			if platformName != "all" && platformName != platformFiles.subDir {
				log.Info().Msgf("Skipping platform %s", platformFiles.subDir)
				continue
			}

			for _, file := range platformFiles.files {
				size, err := parseSize(file.Size)
				if err != nil {
					log.Error().Err(err).Msg("Failed to parse file size")
					return err
				}
				if size > 0 {
					log.Info().Msgf("File: %s, Size: %s", *file.ManualURL, file.Size)
					totalSizeMB += size
				}
			}
		}
	}

	// Calculate the size of extras if included
	if extrasFlag {
		for _, extra := range nestedData.Extras {
			size, err := parseSize(extra.Size)
			if err != nil {
				log.Error().Err(err).Msg("Failed to parse extra size")
				return err
			}
			if size > 0 {
				log.Info().Msgf("Extra: %v, Size: %s", extra.ManualURL, extra.Size)
				totalSizeMB += size
			}
		}
	}

	// Calculate the size of DLCs if included
	if dlcFlag {
		for _, dlc := range nestedData.DLCs {
			for _, download := range dlc.ParsedDownloads {
				if strings.ToLower(download.Language) != strings.ToLower(language) {
					log.Info().Msgf("DLC %s: Skipping language %s", dlc.Title, download.Language)
					continue
				}

				for _, platformFiles := range []struct {
					files  []client.PlatformFile
					subDir string
				}{
					{files: download.Platforms.Windows, subDir: "windows"},
					{files: download.Platforms.Mac, subDir: "mac"},
					{files: download.Platforms.Linux, subDir: "linux"},
				} {
					if platformName != "all" && platformName != platformFiles.subDir {
						log.Info().Msgf("DLC %s: Skipping platform %s", dlc.Title, platformFiles.subDir)
						continue
					}

					for _, file := range platformFiles.files {
						size, err := parseSize(file.Size)
						if err != nil {
							log.Error().Err(err).Msg("Failed to parse file size")
							return err
						}
						if size > 0 {
							log.Info().Msgf("DLC File: %s, Size: %s", *file.ManualURL, file.Size)
							totalSizeMB += size
						}
					}
				}
			}

			// Calculate the size of DLC extras if included
			if extrasFlag {
				for _, extra := range dlc.Extras {
					size, err := parseSize(extra.Size)
					if err != nil {
						log.Error().Err(err).Msg("Failed to parse extra size")
						return err
					}
					if size > 0 {
						log.Info().Msgf("DLC Extra: %v, Size: %s", extra.ManualURL, extra.Size)
						totalSizeMB += size
					}
				}
			}
		}
	}

	// Display the total size in the specified unit
	log.Info().Msgf("Game title: \"%s\"\n", nestedData.Title)
	log.Info().Msgf("Download parameters: Language=%s; Platform=%s; Extras=%t; DLCs=%t\n", language, platformName, extrasFlag, dlcFlag)
	if strings.ToLower(sizeUnit) == "gb" {
		totalSizeGB := totalSizeMB / 1024
		fmt.Printf("Total download size: %.2f GB\n", totalSizeGB)
	} else {
		fmt.Printf("Total download size: %.0f MB\n", totalSizeMB)
	}

	return nil
}
