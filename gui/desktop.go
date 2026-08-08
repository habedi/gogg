package gui

import (
	"os/exec"
	"runtime"

	"github.com/rs/zerolog/log"
)

// folderCommand builds the command that shows path in the system file manager.
// It is a variable so tests can substitute a command.
var folderCommand = func(path string) *exec.Cmd {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", path)
	case "darwin":
		return exec.Command("open", path)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		return exec.Command("xdg-open", path)
	}
}

// openFolder opens the specified path in the system's default file explorer.
// It returns as soon as the file manager has been launched, because it is
// called from the UI thread.
func openFolder(path string) {
	cmd := folderCommand(path)
	if err := cmd.Start(); err != nil {
		log.Error().Err(err).Str("path", path).Msg("Failed to open folder")
		return
	}
	go func() {
		// explorer.exe reports a non-zero exit code even when it succeeds.
		if err := cmd.Wait(); err != nil && runtime.GOOS != "windows" {
			log.Debug().Err(err).Str("path", path).Msg("File manager exited with an error")
		}
	}()
}
