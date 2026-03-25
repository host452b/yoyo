// internal/config/config.go
package config

import (
	"errors"
	"fmt"
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
	Delay *int   // nil = inherit defaults, 0 = immediate, >0 = seconds
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
	return load(path, false)
}

// LoadRequired parses the TOML file at path and returns an error if the file
// does not exist. Use this when the path was explicitly provided by the user.
func LoadRequired(path string) (*Config, error) {
	return load(path, true)
}

func load(path string, required bool) (*Config, error) {
	cfg := &Config{}
	cfg.Defaults.Delay = 3
	cfg.Defaults.Enabled = true
	cfg.Defaults.LogFile = ExpandTilde("~/.yoyo/yoyo.log")

	data, err := os.ReadFile(path)
	if err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	// Apply tilde expansion to paths
	cfg.Defaults.LogFile = ExpandTilde(cfg.Defaults.LogFile)

	// Validate delay values
	if cfg.Defaults.Delay < 0 {
		return nil, fmt.Errorf("defaults.delay must be >= 0, got %d", cfg.Defaults.Delay)
	}
	for name, agent := range cfg.Agents {
		if agent.Delay != nil && *agent.Delay < 0 {
			return nil, fmt.Errorf("agents.%s.delay must be >= 0, got %d", name, *agent.Delay)
		}
	}

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
