package main

import (
	"strings"
	"testing"
)

// ── buildChildEnv tests ─────────────────────────────────────────────────────

// buildChildEnv must replace TERM with xterm-256color for the child PTY.
// The go-pty library creates an xterm-compatible PTY; inheriting a foreign
// TERM (e.g. screen-256color from tmux) causes the child to emit escape
// sequences that the vt10x screen emulator cannot parse, breaking detection.

func TestBuildChildEnv_OverridesTERM(t *testing.T) {
	env := buildChildEnv([]string{
		"HOME=/home/user",
		"TERM=screen-256color",
		"PATH=/usr/bin",
	})

	var found bool
	for _, e := range env {
		if e == "TERM=xterm-256color" {
			found = true
		}
		if e == "TERM=screen-256color" {
			t.Error("original TERM=screen-256color must not appear in child env")
		}
	}
	if !found {
		t.Error("child env must contain TERM=xterm-256color")
	}
}

func TestBuildChildEnv_PreservesOtherVars(t *testing.T) {
	env := buildChildEnv([]string{
		"HOME=/home/user",
		"TERM=screen",
		"CUSTOM=value",
	})

	seen := make(map[string]bool)
	for _, e := range env {
		seen[e] = true
	}
	if !seen["HOME=/home/user"] {
		t.Error("HOME must be preserved")
	}
	if !seen["CUSTOM=value"] {
		t.Error("CUSTOM must be preserved")
	}
}

func TestBuildChildEnv_AddsTERMWhenMissing(t *testing.T) {
	env := buildChildEnv([]string{"HOME=/home/user"})

	var found bool
	for _, e := range env {
		if e == "TERM=xterm-256color" {
			found = true
		}
	}
	if !found {
		t.Error("must add TERM=xterm-256color when TERM is missing from parent env")
	}
}

func TestBuildChildEnv_HandlesEmptyEnv(t *testing.T) {
	env := buildChildEnv(nil)

	if len(env) != 1 || env[0] != "TERM=xterm-256color" {
		t.Errorf("expected [TERM=xterm-256color], got %v", env)
	}
}

func TestBuildChildEnv_StripsTmuxVars(t *testing.T) {
	env := buildChildEnv([]string{
		"HOME=/home/user",
		"TERM=screen-256color",
		"TMUX=/tmp/tmux-1000/default,12345,0",
		"TMUX_PANE=%0",
		"PATH=/usr/bin",
	})

	for _, e := range env {
		if strings.HasPrefix(e, "TMUX=") {
			t.Errorf("child env must not contain TMUX: %s", e)
		}
		if strings.HasPrefix(e, "TMUX_PANE=") {
			t.Errorf("child env must not contain TMUX_PANE: %s", e)
		}
	}
}

func TestBuildChildEnv_DoesNotDuplicateTERM(t *testing.T) {
	env := buildChildEnv([]string{"TERM=tmux-256color", "PATH=/usr/bin"})

	count := 0
	for _, e := range env {
		if strings.HasPrefix(e, "TERM=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 TERM entry, got %d", count)
	}
}

func TestHelpText_Plain_EqualsUsageText(t *testing.T) {
	if helpText(false) != usageText {
		t.Error("plain helpText(false) must be identical to usageText constant")
	}
}

func TestHelpText_Plain_NoANSICodes(t *testing.T) {
	if strings.Contains(helpText(false), "\x1b[") {
		t.Error("plain mode must not contain ANSI escape codes")
	}
}

func TestHelpText_Colored_HasANSICodes(t *testing.T) {
	if !strings.Contains(helpText(true), "\x1b[") {
		t.Error("colored mode must contain ANSI escape codes")
	}
}

func TestHelpText_Colored_SectionHeadersPresent(t *testing.T) {
	text := helpText(true)
	for _, header := range []string{"USAGE", "FLAGS", "EXAMPLES", "RUNTIME CONTROLS"} {
		if !strings.Contains(text, header) {
			t.Errorf("colored help text missing section header %q", header)
		}
	}
}

func TestHelpText_Colored_FlagNamesPresent(t *testing.T) {
	text := helpText(true)
	for _, flag := range []string{"-delay", "-config", "-log", "-dry-run", "-v"} {
		if !strings.Contains(text, flag) {
			t.Errorf("colored help text missing flag %q", flag)
		}
	}
}

func TestHelpText_Colored_AllCodesReset(t *testing.T) {
	text := helpText(true)
	opens := strings.Count(text, "\x1b[")
	resets := strings.Count(text, "\x1b[0m")
	if resets == 0 {
		t.Error("colored help must contain reset codes")
	}
	// Every color open should be followed by a reset somewhere
	if opens < resets {
		t.Errorf("more resets (%d) than color opens (%d)", resets, opens)
	}
}
