package main

import (
	"time"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/config"
)

// cliFlags captures the subset of command-line flags that participate in
// effective-value resolution. Pointer fields use nil to signal "not
// explicitly passed on the command line" — the resolver then falls back
// to per-agent config, then to defaults.
type cliFlags struct {
	Delay       *int           // nil = not explicit
	LogPath     string         // "" = not explicit
	Afk         *bool          // nil = not explicit
	AfkIdle     *time.Duration // nil = not explicit
	Fuzzy       *bool          // nil = not explicit
	FuzzyStable *time.Duration // nil = not explicit
}

// effectiveSettings holds the values actually used by the proxy after
// resolving across defaults → per-agent config → CLI flag.
type effectiveSettings struct {
	Delay       int
	LogFile     string
	Afk         bool
	AfkIdle     time.Duration
	Fuzzy       bool
	FuzzyStable time.Duration
}

// resolveEffective computes the final values by layering sources:
//
//  1. start from cfg.Defaults
//  2. if a per-agent section for kind exists, apply its non-nil overrides,
//     but only if the corresponding CLI flag was NOT passed
//  3. if a CLI flag was explicitly passed, it wins
//
// Hard floors (AfkIdle, FuzzyStable <= 0 → clamp to canonical default)
// are applied last so the returned values are always directly usable.
func resolveEffective(cfg *config.Config, kind agent.Kind, f cliFlags) effectiveSettings {
	s := effectiveSettings{
		Delay:       cfg.Defaults.Delay,
		LogFile:     cfg.Defaults.LogFile,
		Afk:         cfg.Defaults.Afk,
		AfkIdle:     cfg.Defaults.AfkIdle,
		Fuzzy:       cfg.Defaults.Fuzzy,
		FuzzyStable: cfg.Defaults.FuzzyStable,
	}

	if agentCfg, ok := cfg.Agents[kind.String()]; ok {
		if f.Delay == nil && agentCfg.Delay != nil {
			s.Delay = *agentCfg.Delay
		}
		if f.Afk == nil && agentCfg.Afk != nil {
			s.Afk = *agentCfg.Afk
		}
		if f.AfkIdle == nil && agentCfg.AfkIdle != nil {
			s.AfkIdle = *agentCfg.AfkIdle
		}
		if f.Fuzzy == nil && agentCfg.Fuzzy != nil {
			s.Fuzzy = *agentCfg.Fuzzy
		}
		if f.FuzzyStable == nil && agentCfg.FuzzyStable != nil {
			s.FuzzyStable = *agentCfg.FuzzyStable
		}
	}

	if f.Delay != nil {
		s.Delay = *f.Delay
	}
	if f.LogPath != "" {
		s.LogFile = config.ExpandTilde(f.LogPath)
	}
	if f.Afk != nil {
		s.Afk = *f.Afk
	}
	if f.AfkIdle != nil {
		s.AfkIdle = *f.AfkIdle
	}
	if f.Fuzzy != nil {
		s.Fuzzy = *f.Fuzzy
	}
	if f.FuzzyStable != nil {
		s.FuzzyStable = *f.FuzzyStable
	}

	if s.AfkIdle <= 0 {
		s.AfkIdle = 10 * time.Minute
	}
	if s.FuzzyStable <= 0 {
		s.FuzzyStable = 3 * time.Second
	}

	return s
}
