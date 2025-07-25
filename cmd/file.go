package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/habedi/gogg/client"
	"github.com/habedi/gogg/db"
	"github.com/habedi/gogg/pkg/hasher"
	"github.com/habedi/gogg/pkg/pool"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

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
	var numThreads int

	cmd := &cobra.Command{
		Use:   "hash [fileDir]",
		Short: "Generate hash values for game files in a directory",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			dir := args[0]
			if !hasher.IsValidHashAlgo(algo) {
				log.Error().Msgf("Unsupported hash algorithm: %s", algo)
				return
			}
			generateHashFiles(dir, algo, recursiveFlag, saveToFileFlag, cleanFlag, numThreads)
		},
	}
	cmd.Flags().StringVarP(&algo, "algo", "a", "md5", fmt.Sprintf("Hash algorithm to use %v", hasher.HashAlgorithms))
	cmd.Flags().BoolVarP(&recursiveFlag, "recursive", "r", true, "Process files in subdirectories? [true, false]")
	cmd.Flags().BoolVarP(&saveToFileFlag, "save", "s", false, "Save hash to files? [true, false]")
	cmd.Flags().BoolVarP(&cleanFlag, "clean", "c", false, "Remove old hash files before generating new ones? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 4, "Number of worker threads to use for hashing [1-16]")

	return cmd
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
		for _, algo := range hasher.HashAlgorithms {
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

func generateHashFiles(dir, algo string, recursive, saveToFile, clean bool, numThreads int) {
	if clean {
		log.Info().Msgf("Cleaning old hash files from %s and its subdirectories", dir)
		removeHashFiles(dir, true)
	}

	exclusionList := []string{".git", ".gitignore", ".DS_Store", "Thumbs.db", "desktop.ini", "*.json", "*.xml", "*.csv", "*.log", "*.txt", "*.md", "*.html", "*.htm", "*.md5", "*.sha1", "*.sha256", "*.sha512", "*.cksum", "*.sum", "*.sig", "*.asc", "*.gpg"}
	var filesToProcess []string
	walkErr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
		filesToProcess = append(filesToProcess, path)
		return nil
	})
	if walkErr != nil {
		log.Error().Err(walkErr).Msg("Failed to walk directory for hashing")
	}

	var hashFiles []string
	var hfMutex sync.Mutex

	workerFunc := func(ctx context.Context, path string) error {
		hash, err := hasher.GenerateHash(path, algo)
		if err != nil {
			log.Error().Err(err).Str("file", path).Msg("Error generating hash")
			return err
		}
		if saveToFile {
			hashFilePath := path + "." + algo
			err = os.WriteFile(hashFilePath, []byte(hash), 0644)
			if err != nil {
				log.Error().Err(err).Str("file", hashFilePath).Msg("Error writing hash to file")
				return err
			}
			hfMutex.Lock()
			hashFiles = append(hashFiles, hashFilePath)
			hfMutex.Unlock()
		} else {
			fmt.Printf("%s hash for \"%s\": %s\n", algo, path, hash)
		}
		return nil
	}

	_ = pool.Run(context.Background(), filesToProcess, numThreads, workerFunc)

	if saveToFile {
		fmt.Println("Generated hash files:")
		for _, file := range hashFiles {
			fmt.Println(file)
		}
	}
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
	cmd.Flags().StringVarP(&sizeUnit, "unit", "u", "gb", "Size unit to display [gb, mb, kb, b]")
	return cmd
}

func estimateStorageSize(gameID, language, platformName string, extrasFlag, dlcFlag bool, sizeUnit string) error {
	sizeUnit = strings.ToLower(sizeUnit)

	langFullName, ok := client.GameLanguages[language]
	if !ok {
		return fmt.Errorf("invalid language code")
	}

	gameIDInt, err := strconv.Atoi(gameID)
	if err != nil {
		return fmt.Errorf("invalid game ID: %s", gameID)
	}

	game, err := db.GetGameByID(gameIDInt)
	if err != nil {
		return fmt.Errorf("failed to retrieve game data for ID %d: %w", gameIDInt, err)
	}
	if game == nil {
		return fmt.Errorf("game with ID %d not found in the catalogue", gameIDInt)
	}

	var nestedData client.Game
	if err := json.Unmarshal([]byte(game.Data), &nestedData); err != nil {
		return fmt.Errorf("failed to unmarshal game data for ID %d: %w", gameIDInt, err)
	}

	log.Info().Msgf("Game title: \"%s\"\n", nestedData.Title)
	log.Info().Msgf("Download parameters: Language=%s; Platform=%s; Extras=%t; DLCs=%t\n", langFullName, platformName, extrasFlag, dlcFlag)

	totalSizeBytes, err := nestedData.EstimateStorageSize(langFullName, platformName, extrasFlag, dlcFlag)
	if err != nil {
		return fmt.Errorf("failed to calculate storage size: %w", err)
	}

	switch sizeUnit {
	case "gb":
		fmt.Printf("Total download size: %.2f GB\n", float64(totalSizeBytes)/(1024*1024*1024))
	case "mb":
		fmt.Printf("Total download size: %.2f MB\n", float64(totalSizeBytes)/(1024*1024))
	case "kb":
		fmt.Printf("Total download size: %.2f KB\n", float64(totalSizeBytes)/1024)
	case "b":
		fmt.Printf("Total download size: %d B\n", totalSizeBytes)
	default:
		return fmt.Errorf("invalid size unit: \"%s\". Unit must be one of [gb, mb, kb, b]", sizeUnit)
	}

	return nil
}
