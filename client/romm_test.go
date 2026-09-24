package client

import "testing"

// RomM names the Windows platform folder "win"; the other GOG platform names
// already match what RomM expects.
func TestRomMPlatform(t *testing.T) {
	cases := map[string]string{
		"windows":  "win",
		"Windows":  "win",
		" WINDOWS": "win",
		"mac":      "mac",
		"linux":    "linux",
		"all":      "all",
		"":         "",
	}
	for input, want := range cases {
		if got := RomMPlatform(input); got != want {
			t.Errorf("RomMPlatform(%q) = %q, want %q", input, got, want)
		}
	}
}
