// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Duration is a TOML-serialisable wrapper around time.Duration that parses
// strings like "10m" / "1h30m" via UnmarshalText.
type Duration time.Duration

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

type Rule struct {
	Name     string
	Pattern  string
	Response string
}

type AgentConfig struct {
	Delay      *int           // nil = inherit defaults, 0 = immediate, >0 = seconds
	Afk        *bool          // nil = inherit defaults
	AfkIdle    *time.Duration `toml:"-"`
	AfkIdleRaw *Duration      `toml:"afk_idle"`
	Rules      []Rule
}

type Defaults struct {
	Delay   int
	Enabled bool
	Afk     bool
	AfkIdle time.Duration `toml:"-"`
	// AfkIdleRaw is the TOML source; copied into AfkIdle after validation.
	AfkIdleRaw Duration `toml:"afk_idle"`
	LogFile    string   `toml:"log_file"`
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

	// afk_idle: default 10 minutes when unset; negative is invalid.
	if cfg.Defaults.AfkIdleRaw == 0 {
		cfg.Defaults.AfkIdle = 10 * time.Minute
	} else {
		cfg.Defaults.AfkIdle = time.Duration(cfg.Defaults.AfkIdleRaw)
	}
	if cfg.Defaults.AfkIdle < 0 {
		return nil, fmt.Errorf("defaults.afk_idle must be >= 0, got %s", cfg.Defaults.AfkIdle)
	}

	// Validate delay values
	if cfg.Defaults.Delay < 0 {
		return nil, fmt.Errorf("defaults.delay must be >= 0, got %d", cfg.Defaults.Delay)
	}
	for name, agent := range cfg.Agents {
		if agent.Delay != nil && *agent.Delay < 0 {
			return nil, fmt.Errorf("agents.%s.delay must be >= 0, got %d", name, *agent.Delay)
		}
	}

	for name, a := range cfg.Agents {
		if a.AfkIdleRaw != nil {
			d := time.Duration(*a.AfkIdleRaw)
			if d < 0 {
				return nil, fmt.Errorf("agents.%s.afk_idle must be >= 0, got %s", name, d)
			}
			a.AfkIdle = &d
			cfg.Agents[name] = a
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
