package gui

import (
	"os/exec"
	"runtime"

	"github.com/rs/zerolog/log"
)

// openFolder opens the specified path in the system's default file explorer.
func openFolder(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = exec.Command("xdg-open", path)
	}
	if err := cmd.Run(); err != nil {
		log.Error().Err(err).Str("path", path).Msg("Failed to open folder")
	}
}
