package cmd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/habedi/gogg/pkg/hasher"
	"github.com/habedi/gogg/pkg/operations"
	"github.com/habedi/gogg/pkg/validation"
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

			if err := validation.ValidateThreadCount(numThreads); err != nil {
				cmd.PrintErrln("Error:", err)
				return
			}

			if cleanFlag {
				log.Info().Msgf("Cleaning old hash files from %s...", dir)
				if err := operations.CleanHashes(dir, recursiveFlag); err != nil {
					log.Error().Err(err).Msg("Error cleaning old hash files")
				} else {
					log.Info().Msg("Finished cleaning old hash files.")
				}
			}

			files, err := operations.FindFilesToHash(dir, recursiveFlag, operations.DefaultHashExclusions)
			if err != nil {
				log.Error().Err(err).Msg("Error finding files to hash")
				return
			}

			if len(files) == 0 {
				cmd.Println("No files found to hash.")
				return
			}

			cmd.Printf("Found %d files. Generating %s hashes...\n", len(files), algo)
			resultsChan := operations.GenerateHashes(context.Background(), files, algo, numThreads)

			var savedFiles []string
			for res := range resultsChan {
				if res.Err != nil {
					log.Error().Err(res.Err).Str("file", res.File).Msg("Error generating hash")
					continue
				}
				if saveToFileFlag {
					hashFilePath := res.File + "." + algo
					err := os.WriteFile(hashFilePath, []byte(res.Hash), 0644)
					if err != nil {
						log.Error().Err(err).Str("file", hashFilePath).Msg("Error writing hash to file")
					} else {
						savedFiles = append(savedFiles, hashFilePath)
					}
				} else {
					fmt.Printf("%s hash for \"%s\": %s\n", algo, res.File, res.Hash)
				}
			}

			if saveToFileFlag {
				fmt.Println("Generated hash files:")
				for _, file := range savedFiles {
					fmt.Println(file)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&algo, "algo", "a", "md5", fmt.Sprintf("Hash algorithm to use %v", hasher.HashAlgorithms))
	cmd.Flags().BoolVarP(&recursiveFlag, "recursive", "r", true, "Process files in subdirectories? [true, false]")
	cmd.Flags().BoolVarP(&saveToFileFlag, "save", "s", false, "Save hash to files? [true, false]")
	cmd.Flags().BoolVarP(&cleanFlag, "clean", "c", false, "Remove old hash files before generating new ones? [true, false]")
	cmd.Flags().IntVarP(&numThreads, "threads", "t", 4, "Number of worker threads to use for hashing [1-16]")

	return cmd
}

func sizeCmd() *cobra.Command {
	var language, platformName, sizeUnit string
	var extrasFlag, dlcFlag bool

	cmd := &cobra.Command{
		Use:   "size [gameID]",
		Short: "Show the total storage size needed to download game files",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			gameID, err := strconv.Atoi(args[0])
			if err != nil {
				cmd.PrintErrln("Error: Invalid game ID. It must be a positive integer.")
				return
			}

			if err := validation.ValidateGameID(gameID); err != nil {
				cmd.PrintErrln("Error:", err)
				return
			}

			params := operations.EstimationParams{
				LanguageCode:  strings.ToLower(language),
				PlatformName:  platformName,
				IncludeExtras: extrasFlag,
				IncludeDLCs:   dlcFlag,
			}

			totalSizeBytes, gameData, err := operations.EstimateGameSize(gameID, params)
			if err != nil {
				log.Error().Err(err).Msg("Error estimating storage size")
				cmd.PrintErrln("Error:", err)
				return
			}

			log.Info().Msgf("Game title: \"%s\"\n", gameData.Title)
			log.Info().Msgf("Download parameters: Language=%s; Platform=%s; Extras=%t; DLCs=%t\n", params.LanguageCode, params.PlatformName, params.IncludeExtras, params.IncludeDLCs)

			sizeUnit = strings.ToLower(sizeUnit)
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
				cmd.PrintErrf("invalid size unit: %q. Unit must be one of [gb, mb, kb, b]\n", sizeUnit)
				return
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
