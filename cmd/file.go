package cmd

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var hashAlgorithms = []string{"md5", "sha1", "sha256", "sha512"}

func fileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "Perform various file operations",
	}
	cmd.AddCommand(hashCmd(), sizeCmd())
	return cmd
}

func hashCmd() *cobra.Command {
	var saveToFileFlag, cleanFlag, recursiveFlag bool
	var algo string

	cmd := &cobra.Command{
		Use:   "hash [fileDir]",
		Short: "Generate hash values for game files in a directory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			if !isValidHashAlgo(algo) {
				log.Error().Msgf("Unsupported hash algorithm: %s", algo)
				return
			}
			generateHashFiles(dir, algo, recursiveFlag, saveToFileFlag, cleanFlag)
		},
	}
	cmd.Flags().StringVarP(&algo, "algo", "a", "md5", "Hash algorithm to use [md5, sha1, sha256, sha512]")
	cmd.Flags().BoolVarP(&recursiveFlag, "recursive", "r", true, "Process files in subdirectories? [true, false]")
	cmd.Flags().BoolVarP(&saveToFileFlag, "save", "s", false, "Save hash to files? [true, false]")
	cmd.Flags().BoolVarP(&cleanFlag, "clean", "c", false, "Remove old hash files before generating new ones? [true, false]")
	return cmd
}

func isValidHashAlgo(algo string) bool {
	for _, validAlgo := range hashAlgorithms {
		if strings.ToLower(algo) == validAlgo {
			return true
		}
	}
	return false
}

func removeHashFiles(dir string, recursive bool) {
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Error().Msgf("Error accessing path %q: %v", path, err)
			return err
		}
		if info.IsDir() && !recursive {
			return filepath.SkipDir
		}
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

func generateHashFiles(dir, algo string, recursive, saveToFile, clean bool) {
	exclusionList := []string{".git", ".gitignore", ".DS_Store", "Thumbs.db", "desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm", "*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg"}
	var hashFiles []string
	var wg sync.WaitGroup
	fileChan := make(chan string)
	numWorkers := runtime.NumCPU() - 2
	if numWorkers < 2 {
		numWorkers = 2
	}
	if clean {
		log.Info().Msgf("Cleaning old hash files from %s and its subdirectories", dir)
		removeHashFiles(dir, true)
	}
	go func() {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				log.Error().Msgf("Error accessing path %q: %v", path, err)
				return err
			}
			if info.IsDir() {
				if path != dir && !recursive {
					return filepath.SkipDir
				}
				return nil
			}
			for _, pattern := range exclusionList {
				matched, _ := filepath.Match(pattern, info.Name())
				if matched {
					return nil
				}
			}
			fileChan <- path
			return nil
		})
		if err != nil {
			log.Error().Msgf("Error walking the path %q: %v", dir, err)
		}
		close(fileChan)
	}()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				hash, err := generateHash(path, algo)
				if err != nil {
					log.Error().Msgf("Error generating hash for file %s: %v", path, err)
					continue
				}
				if saveToFile {
					hashFilePath := path + "." + algo
					err = os.WriteFile(hashFilePath, []byte(hash), 0o644)
					if err != nil {
						log.Error().Msgf("Error writing hash to file %s: %v", hashFilePath, err)
						continue
					}
					hashFiles = append(hashFiles, hashFilePath)
				} else {
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

func generateHash(filePath, algo string) (string, error) {
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

func sizeCmd() *cobra.Command {
	var language, platformName, sizeUnit string
	var extrasFlag, dlcFlag bool

	cmd := &cobra.Command{
		Use:   "size [gameID]",
		Short: "Show the total storage size needed to download game files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			err := estimateStorageSize(args[0], strings.ToLower(language), platformName, extrasFlag, dlcFlag, sizeUnit)
			if err != nil {
				log.Fatal().Err(err).Msg("Error estimating storage size for game files to be downloaded")
			}
		},
	}
	cmd.Flags().StringVarP(&language, "lang", "l", "en", "Game language [en, fr, de, es, it, ru, pl, pt-BR, zh-Hans, ja, ko]")
	cmd.Flags().StringVarP(&platformName, "platform", "p", "windows", "Platform name [all, windows, mac, linux]; all means all platforms")
	cmd.Flags().BoolVarP(&extrasFlag, "extras", "e", true, "Include extra content files? [true, false]")
	cmd.Flags().BoolVarP(&dlcFlag, "dlcs", "d", true, "Include DLC files? [true, false]")
	cmd.Flags().StringVarP(&sizeUnit, "unit", "u", "mb", "Size unit to display [mb, gb]")
	return cmd
}

func estimateStorageSize(gameID, language, platformName string, extrasFlag, dlcFlag bool, sizeUnit string) error {
	sizeUnit = strings.ToLower(sizeUnit)
	if sizeUnit != "mb" && sizeUnit != "gb" {
		fmt.Printf("Invalid size unit: \"%s\". Unit must be mb or gb\n", sizeUnit)
		return fmt.Errorf("invalid size unit")
	}
	if !isValidLanguage(language) {
		fmt.Println("Invalid language code. Supported languages are:")
		for langCode, langName := range gameLanguages {
			fmt.Printf("'%s' for %s\n", langCode, langName)
		}
		return fmt.Errorf("invalid language code")
	} else {
		language = gameLanguages[language]
	}
	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		log.Error().Msgf("Invalid game ID: %s", gameID)
		return err
	}
	game, err := db.GetGameByID(gameIDInt)
	if err != nil {
		log.Error().Msgf("Failed to retrieve game data for ID %d: %v", gameIDInt, err)
		return err
	}
	if game == nil {
		log.Error().Msgf("Game not found for ID %d", gameIDInt)
		fmt.Printf("Game with ID %d not found in the catalogue.\n", gameIDInt)
		return err
	}
	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		log.Error().Msgf("Failed to unmarshal game data for ID %d: %v", gameIDInt, err)
		return err
	}
	var totalSizeMB float64
	parseSize := func(sizeStr string) (float64, error) {
		sizeStr = strings.TrimSpace(strings.ToLower(sizeStr))
		if strings.HasSuffix(sizeStr, " gb") {
			sizeStr = strings.TrimSuffix(sizeStr, " gb")
			size, err := strconv.ParseFloat(sizeStr, 64)
			if err != nil {
				return 0, err
			}
			return size * 1024, nil
		} else if strings.HasSuffix(sizeStr, " mb") {
			sizeStr = strings.TrimSuffix(sizeStr, " mb")
			return strconv.ParseFloat(sizeStr, 64)
		}
		return 0, fmt.Errorf("unknown size unit")
	}
	for _, download := range nestedData.Downloads {
		if !strings.EqualFold(download.Language, language) {
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
	if dlcFlag {
		for _, dlc := range nestedData.DLCs {
			for _, download := range dlc.ParsedDownloads {
				if !strings.EqualFold(download.Language, language) {
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
