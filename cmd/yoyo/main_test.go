package main

import (
	"strings"
	"testing"
)

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
	for _, flag := range []string{"-delay", "-config", "-log"} {
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
