package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds user-defined defaults for gogg commands.
// Values here act as flag defaults; explicit CLI flags always take precedence.
type Config struct {
	Language    string `json:"language"`
	Platform    string `json:"platform"`
	DownloadDir string `json:"download_dir"`
	Extras      bool   `json:"extras"`
	DLCs        bool   `json:"dlcs"`
	Resume      bool   `json:"resume"`
	Threads     int    `json:"threads"`
	// Connections is how many HTTP connections may fetch one file at once.
	Connections int  `json:"connections"`
	Flatten     bool `json:"flatten"`
	SkipPatches bool `json:"skip_patches"`
	KeepLatest  bool `json:"keep_latest"`
	RommLayout  bool `json:"romm_layout"`
	// LutrisLayout arranges downloads the way Lutris caches installers.
	LutrisLayout bool `json:"lutris_layout"`
	// NoVerify turns off checking downloaded files against the MD5 GOG
	// publishes. The key is negative so that a config written before it
	// existed, which decodes to false, keeps verification on.
	NoVerify bool `json:"no_verify"`
}

// Defaults returns the built-in default configuration.
func Defaults() Config {
	return Config{
		Language:    "en",
		Platform:    "windows",
		Extras:      true,
		DLCs:        true,
		Resume:      true,
		Threads:     5,
		Connections: 1,
		Flatten:     true,
		SkipPatches: true,
	}
}

// Path returns the path to the config file (~/.config/gogg/config.json).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gogg", "config.json"), nil
}

// Load reads the config file and merges it over the built-in defaults.
// If the file does not exist or cannot be parsed, built-in defaults are returned.
func Load() Config {
	cfg := Defaults()
	p, err := Path()
	if err != nil {
		return cfg
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// Save writes cfg to the config file, creating the directory if needed.
func Save(cfg Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}
