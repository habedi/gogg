package hasher

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"strings"
)

// HashAlgorithms is a list of supported hashing algorithms.
var HashAlgorithms = []string{"md5", "sha1", "sha256", "sha512"}

// IsValidHashAlgo checks if the provided algorithm string is supported.
func IsValidHashAlgo(algo string) bool {
	for _, validAlgo := range HashAlgorithms {
		if strings.ToLower(algo) == validAlgo {
			return true
		}
	}
	return false
}

// GenerateHashFromReader calculates the hash of content from an io.Reader.
func GenerateHashFromReader(reader io.Reader, algo string) (string, error) {
	var h hash.Hash
	switch strings.ToLower(algo) {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	default:
		return "", fmt.Errorf("unsupported hash algorithm: %s", algo)
	}

	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}
