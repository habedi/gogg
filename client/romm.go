package client

import "strings"

// RomMPlatform converts a GOG platform name into the folder name RomM
// expects. RomM names the Windows platform folder "win"; the mac and linux
// names already match. Other values pass through lowercased so unknown
// platforms still produce a usable folder name.
func RomMPlatform(platform string) string {
	name := strings.ToLower(strings.TrimSpace(platform))
	if name == "windows" {
		return "win"
	}
	return name
}
