// internal/agent/agent_test.go
package agent_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/agent"
	"github.com/host452b/yoyo/internal/detector"
)

func TestKindFromCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want agent.Kind
	}{
		{"claude", agent.KindClaude},
		{"/usr/bin/claude", agent.KindClaude},
		{"claude.exe", agent.KindClaude},
		{"codex", agent.KindCodex},
		{"cursor", agent.KindCursor},
		{"unknown-tool", agent.KindUnknown},
		{"", agent.KindUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.cmd, func(t *testing.T) {
			got := agent.KindFromCommand(tc.cmd)
			if got != tc.want {
				t.Errorf("KindFromCommand(%q) = %v, want %v", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestKindFromScreen(t *testing.T) {
	tests := []struct {
		text string
		want agent.Kind
	}{
		{"Welcome to Claude Code", agent.KindClaude},
		{"Codex CLI v1.0", agent.KindCodex},
		{"Codex CLI", agent.KindCodex},
		{"codex@0.1.0", agent.KindCodex},
		{"Cursor Agent ready", agent.KindCursor},
		{"cursor-agent running", agent.KindCursor},
		{"something else", agent.KindUnknown},
	}
	for _, tc := range tests {
		t.Run(tc.text, func(t *testing.T) {
			got := agent.KindFromScreen(tc.text)
			if got != tc.want {
				t.Errorf("KindFromScreen(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

func TestKind_Detector_Claude(t *testing.T) {
	d := agent.KindClaude.Detector()
	if d == nil {
		t.Error("expected non-nil detector for Claude")
	}
	// Should behave as Claude detector
	prompt := "─────────────\n Read file\n 1. Yes\n 2. No\n Esc to cancel\n"
	if d.Detect(prompt) == nil {
		t.Error("Claude kind detector should detect Claude prompts")
	}
}

func TestKind_Detector_Unknown_TriesAll(t *testing.T) {
	d := agent.KindUnknown.Detector()
	// Should detect Claude prompts
	claudePrompt := "─────────────\n Read file\n 1. Yes\n 2. No\n Esc to cancel\n"
	if d.Detect(claudePrompt) == nil {
		t.Error("Unknown kind should try Claude detector")
	}
	// Should detect Codex prompts
	codexPrompt := "  Would you like to run this?\n› 1. Yes\n" +
		"  Press enter to confirm or esc to cancel\n"
	if d.Detect(codexPrompt) == nil {
		t.Error("Unknown kind should try Codex detector")
	}
}

// Ensure agent.Kind satisfies detector.Detector interface
var _ detector.Detector = agent.Kind(0)
