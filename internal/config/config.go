// Package config loads and saves the app's TOML configuration: which ISOs
// are enabled, which variants are selected, and general preferences.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// Config is the root of the persisted configuration file.
type Config struct {
	General General              `toml:"general"`
	ISO     map[string]ISOConfig `toml:"iso"`
}

// General holds app-wide preferences.
type General struct {
	DownloadDir string `toml:"download_dir"`
	Theme       string `toml:"theme"`    // "light", "dark", or "auto"
	Language    string `toml:"language"` // "fr" or "en"
}

// ISOConfig holds per-provider settings, keyed by provider ID in Config.ISO.
// DisabledVariants indexes into that provider's Variants() slice — a variant
// not listed there is visible. An empty/missing list means every variant is
// visible, which is the default for any provider the user hasn't touched.
type ISOConfig struct {
	DisabledVariants []int  `toml:"disabled_variants,omitempty"`
	DownloadDir      string `toml:"download_dir,omitempty"`
}

// Default returns the configuration used when no file exists yet.
func Default() Config {
	dir, err := os.UserHomeDir()
	if err != nil {
		dir = "."
	}
	return Config{
		General: General{
			DownloadDir: filepath.Join(dir, "Downloads", "ISOs"),
			Theme:       "auto",
			Language:    "en",
		},
		ISO: map[string]ISOConfig{},
	}
}

// DefaultPath returns the standard config file location:
// ~/Library/Application Support/iso-auto-downloader/config.toml on macOS.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config: resolve user config dir: %w", err)
	}
	return filepath.Join(dir, "iso-auto-downloader", "config.toml"), nil
}

// Load reads the config at path, returning Default() if the file doesn't exist.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	cfg := Default()
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path atomically (write to a temp file, then rename),
// so a crash or full disk mid-write can't corrupt the existing config.
func Save(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("config: create dir for %s: %w", path, err)
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("config: encode: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("config: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("config: finalize %s: %w", path, err)
	}
	return nil
}
