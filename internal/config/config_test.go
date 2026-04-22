// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.toml")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(content)
	f.Close()
	return f.Name()
}

func TestLoad_Defaults(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Delay != 3 {
		t.Errorf("default delay = %d, want 3", cfg.Defaults.Delay)
	}
	if !cfg.Defaults.Enabled {
		t.Error("default enabled = false, want true")
	}
}

func TestLoad_AgentDelay(t *testing.T) {
	path := writeConfig(t, `
[agents.claude]
delay = 1
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	agentDelay := cfg.Agents["claude"].Delay
	if agentDelay == nil || *agentDelay != 1 {
		var got interface{} = "<nil>"
		if agentDelay != nil {
			got = *agentDelay
		}
		t.Errorf("claude delay = %v, want 1", got)
	}
}

func TestLoad_AgentDelayInheritsDefault(t *testing.T) {
	// Agent section without delay field must NOT override global default.
	path := writeConfig(t, `
[agents.claude]
# no delay set — should inherit defaults.delay
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["claude"].Delay != nil {
		t.Errorf("claude delay should be nil (inherit default), got %d", *cfg.Agents["claude"].Delay)
	}
}

func TestLoad_GlobalRule(t *testing.T) {
	path := writeConfig(t, `
[[rules]]
name = "confirm"
pattern = "Are you sure"
response = "y\r"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("rules count = %d, want 1", len(cfg.Rules))
	}
	if cfg.Rules[0].Name != "confirm" {
		t.Errorf("rule name = %q, want 'confirm'", cfg.Rules[0].Name)
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	path := writeConfig(t, `
[defaults]
log_file = "~/yoyo.log"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, "yoyo.log")
	if cfg.Defaults.LogFile != want {
		t.Errorf("log_file = %q, want %q", cfg.Defaults.LogFile, want)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path.toml")
	if err != nil {
		t.Fatal("expected no error for missing config, got:", err)
	}
	// Missing file returns defaults
	if cfg.Defaults.Delay != 3 {
		t.Errorf("delay = %d, want 3", cfg.Defaults.Delay)
	}
}

func TestLoad_NegativeDelay_ReturnsError(t *testing.T) {
	path := writeConfig(t, "[defaults]\ndelay = -1\n")
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for negative delay in config, got nil")
	}
}

func TestLoad_NegativeAgentDelay_ReturnsError(t *testing.T) {
	path := writeConfig(t, "[agents.claude]\ndelay = -2\n")
	_, err := config.Load(path)
	if err == nil {
		t.Error("expected error for negative agent delay in config, got nil")
	}
}

func TestLoadRequired_MissingFile_ReturnsError(t *testing.T) {
	_, err := config.LoadRequired("/nonexistent/explicit/path.toml")
	if err == nil {
		t.Error("expected error for explicitly-required missing config, got nil")
	}
}

func TestLoadRequired_ExistingFile_Works(t *testing.T) {
	path := writeConfig(t, "[defaults]\ndelay = 2\n")
	cfg, err := config.LoadRequired(path)
	if err != nil {
		t.Fatal("expected no error:", err)
	}
	if cfg.Defaults.Delay != 2 {
		t.Errorf("delay = %d, want 2", cfg.Defaults.Delay)
	}
}

func TestLoad_AfkDefaultsOff(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Afk {
		t.Error("default afk = true, want false")
	}
	if cfg.Defaults.AfkIdle != 10*time.Minute {
		t.Errorf("default afk_idle = %v, want 10m", cfg.Defaults.AfkIdle)
	}
}

func TestLoad_AfkExplicit(t *testing.T) {
	path := writeConfig(t, `
[defaults]
afk      = true
afk_idle = "2m"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Defaults.Afk {
		t.Error("afk = false, want true")
	}
	if cfg.Defaults.AfkIdle != 2*time.Minute {
		t.Errorf("afk_idle = %v, want 2m", cfg.Defaults.AfkIdle)
	}
}

func TestLoad_AfkIdle_Negative_ReturnsError(t *testing.T) {
	path := writeConfig(t, `
[defaults]
afk_idle = "-30s"
`)
	if _, err := config.Load(path); err == nil {
		t.Error("expected error for negative afk_idle, got nil")
	}
}

func TestLoad_AgentAfkOverride(t *testing.T) {
	path := writeConfig(t, `
[agents.claude]
afk      = true
afk_idle = "3m"
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ac := cfg.Agents["claude"]
	if ac.Afk == nil || !*ac.Afk {
		t.Error("claude.afk should be explicitly true")
	}
	if ac.AfkIdle == nil || *ac.AfkIdle != 3*time.Minute {
		t.Errorf("claude.afk_idle = %v, want 3m", *ac.AfkIdle)
	}
}

func TestLoad_AgentAfkOmitted_Nil(t *testing.T) {
	path := writeConfig(t, `
[agents.claude]
delay = 1
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ac := cfg.Agents["claude"]
	if ac.Afk != nil {
		t.Error("omitted afk should parse as nil (inherit)")
	}
	if ac.AfkIdle != nil {
		t.Error("omitted afk_idle should parse as nil (inherit)")
	}
}
