package validation

import (
	"fmt"
)

const (
	MinThreads = 1
	MaxThreads = 20
)

func ValidateThreadCount(threads int) error {
	if threads < MinThreads || threads > MaxThreads {
		return fmt.Errorf("thread count must be between %d and %d, got %d", MinThreads, MaxThreads, threads)
	}
	return nil
}

func ValidateGameID(id int) error {
	if id <= 0 {
		return fmt.Errorf("game ID must be a positive integer, got %d", id)
	}
	return nil
}

func ValidateNonEmptyString(fieldName, value string) error {
	if value == "" {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

func ValidateLanguageCode(code string, validCodes map[string]string) error {
	if _, ok := validCodes[code]; !ok {
		return fmt.Errorf("invalid language code: %s", code)
	}
	return nil
}

func ValidatePlatform(platform string) error {
	validPlatforms := map[string]bool{
		"all":     true,
		"windows": true,
		"mac":     true,
		"linux":   true,
	}
	if !validPlatforms[platform] {
		return fmt.Errorf("invalid platform: %s (must be one of: all, windows, mac, linux)", platform)
	}
	return nil
}
