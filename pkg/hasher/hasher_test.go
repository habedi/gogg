package hasher_test

import (
	"strings"
	"testing"

	"github.com/habedi/gogg/pkg/hasher"
	"github.com/stretchr/testify/assert"
)

func TestIsValidHashAlgo(t *testing.T) {
	assert.True(t, hasher.IsValidHashAlgo("md5"))
	assert.True(t, hasher.IsValidHashAlgo("sha1"))
	assert.True(t, hasher.IsValidHashAlgo("sha256"))
	assert.True(t, hasher.IsValidHashAlgo("sha512"))
	assert.True(t, hasher.IsValidHashAlgo("SHA1"))
	assert.False(t, hasher.IsValidHashAlgo("md4"))
	assert.False(t, hasher.IsValidHashAlgo(""))
}

func TestGenerateHashFromReader(t *testing.T) {
	content := "hello world"

	testCases := []struct {
		algo     string
		expected string
		wantErr  bool
	}{
		{"md5", "5eb63bbbe01eeed093cb22bb8f5acdc3", false},
		{"sha1", "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed", false},
		{"sha256", "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9", false},
		{"sha512", "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f", false},
		{"invalid", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.algo, func(t *testing.T) {
			reader := strings.NewReader(content)
			hash, err := hasher.GenerateHashFromReader(reader, tc.algo)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expected, hash)
			}
		})
	}
}
