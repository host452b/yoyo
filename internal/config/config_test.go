// internal/config/config_test.go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

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
