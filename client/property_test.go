package client

import (
	"math/rand"
	"testing"
)

func TestFuzzSanitizePath(t *testing.T) {
	r := rand.New(rand.NewSource(42))
	chars := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 -_()[]{}!@#$%^&*+=|\\/<>?\"'™®:")
	for i := 0; i < 1000; i++ {
		l := r.Intn(64)
		runes := make([]rune, l)
		for j := 0; j < l; j++ {
			runes[j] = chars[r.Intn(len(chars))]
		}
		in := string(runes)
		out := SanitizePath(in)
		if out != "" {
			if out[0] == '-' || out[len(out)-1] == '-' {
				t.Fatalf("sanitize produced leading/trailing hyphen for %q -> %q", in, out)
			}
			for k := 0; k < len(out); k++ {
				c := out[k]
				if c == '/' || c == '\\' {
					t.Fatalf("sanitize produced path separator for %q -> %q", in, out)
				}
			}
		}
	}
}

func TestFuzzParseSizeString(t *testing.T) {
	cases := []string{"1", "1024", "1KB", "1 kb", "1.5MB", "2 mb", "10GB", "0.1 gb", "3TB", "1GiB", "invalid"}
	for _, c := range cases {
		_, _ = parseSizeString(c)
	}
}
