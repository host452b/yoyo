// internal/config/config.go
package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type Rule struct {
	Name     string
	Pattern  string
	Response string
}

type AgentConfig struct {
	Delay int    // -1 = inherit defaults
	Rules []Rule
}

type Defaults struct {
	Delay   int
	Enabled bool
	LogFile string `toml:"log_file"`
}

type Config struct {
	Defaults Defaults
	Agents   map[string]AgentConfig
	Rules    []Rule
}

// Load parses the TOML file at path and applies defaults.
// Returns defaults if the file does not exist.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	cfg.Defaults.Delay = 3
	cfg.Defaults.Enabled = true
	cfg.Defaults.LogFile = ExpandTilde("~/.yoyo/yoyo.log")

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply tilde expansion to paths
	cfg.Defaults.LogFile = ExpandTilde(cfg.Defaults.LogFile)

	return cfg, nil
}

// DefaultPath returns the default config file path (~/.config/yoyo/config.toml).
func DefaultPath() string {
	return ExpandTilde("~/.config/yoyo/config.toml")
}

// ExpandTilde expands a leading "~/" to the user's home directory.
func ExpandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}
