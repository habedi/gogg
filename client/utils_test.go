package client

import "testing"

func Test_parseSizeString(t *testing.T) {
	cases := []struct {
		in  string
		ok  bool
		min int64
	}{
		{"1024", true, 1024},
		{"1KB", true, 1024},
		{"1 KB", true, 1024},
		{"1.5 MB", true, 1_500_000},
		{"2mb", true, 2 * 1024 * 1024},
		{"0.5GB", true, 512 * 1024 * 1024},
		{"1GiB", true, 1024 * 1024 * 1024},
		{"10 tb", true, 10 * 1024 * 1024 * 1024 * 1024},
		{"unknown", false, 0},
	}
	for _, c := range cases {
		v, err := parseSizeString(c.in)
		if c.ok {
			if err != nil {
				t.Fatalf("expected ok for %q: %v", c.in, err)
			}
			if v < c.min {
				t.Fatalf("expected %q >= %d, got %d", c.in, c.min, v)
			}
		} else if err == nil {
			t.Fatalf("expected error for %q", c.in)
		}
	}
}
