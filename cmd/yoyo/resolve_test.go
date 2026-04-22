package main

import (
	"testing"
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/config"
)

// baseConfig returns a minimal Defaults-only config with sensible seeds.
func baseConfig() *config.Config {
	return &config.Config{
		Defaults: config.Defaults{
			Delay:       3,
			Enabled:     true,
			Afk:         false,
			AfkIdle:     10 * time.Minute,
			Fuzzy:       false,
			FuzzyStable: 3 * time.Second,
			LogFile:     "/tmp/yoyo.log",
		},
		Agents: map[string]config.AgentConfig{},
	}
}

func intp(v int) *int                     { return &v }
func boolp(v bool) *bool                  { return &v }
func durp(v time.Duration) *time.Duration { return &v }

func TestResolveEffective_DefaultsOnly(t *testing.T) {
	cfg := baseConfig()
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{})
	if got.Delay != 3 || got.Afk != false || got.AfkIdle != 10*time.Minute ||
		got.Fuzzy != false || got.FuzzyStable != 3*time.Second ||
		got.LogFile != "/tmp/yoyo.log" {
		t.Errorf("defaults not preserved: %+v", got)
	}
}

func TestResolveEffective_FlagBeatsDefaults(t *testing.T) {
	cfg := baseConfig()
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{
		Delay:       intp(1),
		Afk:         boolp(true),
		AfkIdle:     durp(30 * time.Second),
		Fuzzy:       boolp(true),
		FuzzyStable: durp(5 * time.Second),
	})
	if got.Delay != 1 || !got.Afk || got.AfkIdle != 30*time.Second ||
		!got.Fuzzy || got.FuzzyStable != 5*time.Second {
		t.Errorf("flag didn't win over defaults: %+v", got)
	}
}

func TestResolveEffective_AgentBeatsDefaultsWhenFlagAbsent(t *testing.T) {
	cfg := baseConfig()
	cfg.Agents["claude"] = config.AgentConfig{
		Delay:       intp(0),
		Afk:         boolp(true),
		AfkIdle:     durp(2 * time.Minute),
		Fuzzy:       boolp(true),
		FuzzyStable: durp(1 * time.Second),
	}
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{})
	if got.Delay != 0 || !got.Afk || got.AfkIdle != 2*time.Minute ||
		!got.Fuzzy || got.FuzzyStable != 1*time.Second {
		t.Errorf("agent override not applied: %+v", got)
	}
}

func TestResolveEffective_FlagBeatsAgent(t *testing.T) {
	cfg := baseConfig()
	cfg.Agents["claude"] = config.AgentConfig{
		Delay:   intp(0),
		Afk:     boolp(true),
		AfkIdle: durp(2 * time.Minute),
	}
	// Flag explicitly sets different values
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{
		Delay:   intp(5),
		Afk:     boolp(false),
		AfkIdle: durp(20 * time.Minute),
	})
	if got.Delay != 5 || got.Afk != false || got.AfkIdle != 20*time.Minute {
		t.Errorf("flag didn't beat agent: %+v", got)
	}
}

func TestResolveEffective_AgentForDifferentKind_NotApplied(t *testing.T) {
	cfg := baseConfig()
	cfg.Agents["codex"] = config.AgentConfig{
		Delay: intp(99),
		Afk:   boolp(true),
	}
	// Resolving for claude — codex overrides must NOT leak.
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{})
	if got.Delay != 3 || got.Afk != false {
		t.Errorf("codex override leaked into claude: %+v", got)
	}
}

func TestResolveEffective_AgentPartialOverride(t *testing.T) {
	// Only AfkIdle set on agent — Afk bool should still come from defaults.
	cfg := baseConfig()
	cfg.Defaults.Afk = true
	cfg.Agents["claude"] = config.AgentConfig{
		AfkIdle: durp(1 * time.Minute),
	}
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{})
	if !got.Afk {
		t.Error("defaults.afk=true should survive partial per-agent override")
	}
	if got.AfkIdle != 1*time.Minute {
		t.Errorf("agent AfkIdle not applied: got %v", got.AfkIdle)
	}
}

func TestResolveEffective_LogPathFlagExpandsTilde(t *testing.T) {
	cfg := baseConfig()
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{LogPath: "/var/log/yoyo.log"})
	if got.LogFile != "/var/log/yoyo.log" {
		t.Errorf("LogFile = %q, want '/var/log/yoyo.log'", got.LogFile)
	}
}

func TestResolveEffective_ClampsNonPositiveDurations(t *testing.T) {
	cfg := baseConfig()
	// Pretend config loaded with a zero AfkIdle (shouldn't happen via real
	// TOML parse due to the load()-time default, but defensive clamp lives
	// here too). Flag explicitly passes 0 → must clamp, not disable.
	got := resolveEffective(cfg, agent.KindClaude, cliFlags{
		AfkIdle:     durp(0),
		FuzzyStable: durp(0),
	})
	if got.AfkIdle != 10*time.Minute {
		t.Errorf("AfkIdle not clamped: got %v", got.AfkIdle)
	}
	if got.FuzzyStable != 3*time.Second {
		t.Errorf("FuzzyStable not clamped: got %v", got.FuzzyStable)
	}
}

func TestResolveEffective_UnknownAgentKind_NoOverrides(t *testing.T) {
	cfg := baseConfig()
	cfg.Agents["claude"] = config.AgentConfig{Delay: intp(0)}
	// KindUnknown has no entry in the map — should use defaults only.
	got := resolveEffective(cfg, agent.KindUnknown, cliFlags{})
	if got.Delay != 3 {
		t.Errorf("unknown kind picked up claude override: got delay=%d", got.Delay)
	}
}
