// internal/detector/claude_test.go
package detector_test

import (
	"testing"

	"github.com/host452b/yoyo/internal/detector"
)

func claudePrompt(action string, options []string) string {
	s := "─────────────────────────────────────────────\n"
	s += " " + action + "\n\n"
	for i, opt := range options {
		s += "   " + string(rune('1'+i)) + ". " + opt + "\n"
	}
	s += "\n Esc to cancel · Tab to amend\n"
	return s
}

func TestClaude_DetectsPrompt(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	if d.Detect(p) == nil {
		t.Error("expected detection, got nil")
	}
}

func TestClaude_StripsWhitespaceAndSpecialChars(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────\n  ❯ Read file  \n\n   1. Yes\n   2. No\n\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.RuleName != "Claude" {
		t.Errorf("RuleName = %q, want 'Claude'", r.RuleName)
	}
}

func TestClaude_FullExample(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────────────────────────────\n" +
		" Read file\n\n" +
		"  Read(/some/path/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n" +
		"   2. Yes, allow reading from src/ during this session\n" +
		"   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Hash == "" {
		t.Error("expected non-empty hash")
	}
}

func TestClaude_NoSeparator(t *testing.T) {
	d := detector.Claude{}
	if d.Detect(" Read file\n\n Esc to cancel\n") != nil {
		t.Error("should not detect without separator")
	}
}

func TestClaude_NoEscLineNoNoOption(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n 1. Yes\n") != nil {
		t.Error("should not detect without No option")
	}
}

func TestClaude_FallbackNoEscWithNoOption(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Read file\n\n 1. Yes\n 2. No\n"
	if d.Detect(p) == nil {
		t.Error("should detect with fallback bottom boundary")
	}
}

func TestClaude_FallbackLongBodyNoEsc(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────────────────────────────\n" +
		" Read file\n\n  Read(/some/long/path.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. Yes, allow reading from src/ during this session\n   3. No\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_IncompleteRenderNoOptions(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n\n Esc to cancel\n") != nil {
		t.Error("should not detect incomplete render (no options)")
	}
}

func TestClaude_IncompleteRenderPartialOptions(t *testing.T) {
	d := detector.Claude{}
	if d.Detect("─────────────\n Read file\n 1. Yes\n Esc to cancel\n") != nil {
		t.Error("should not detect when only Yes rendered but No missing")
	}
}

func TestClaude_PicksLastSeparatorWhenMultiple(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────\n old output with separator\n" +
		"─────────────\n Write file\n 1. Yes\n 2. No\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_StableAcrossRedraws(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	r1 := d.Detect(p)
	r2 := d.Detect(p)
	if r1 == nil || r2 == nil {
		t.Fatal("expected both detections to succeed")
	}
	if r1.Hash != r2.Hash {
		t.Error("hash should be stable across identical redraws")
	}
}

func TestClaude_DifferentPromptsDiffer(t *testing.T) {
	d := detector.Claude{}
	r1 := d.Detect(claudePrompt("Read file", []string{"Yes", "No"}))
	r2 := d.Detect(claudePrompt("Write file", []string{"Yes", "No"}))
	if r1 == nil || r2 == nil {
		t.Fatal("both should be detected")
	}
	if r1.Hash == r2.Hash {
		t.Error("different prompts should have different hashes")
	}
}

func TestClaude_SeparatorScrolledOff(t *testing.T) {
	d := detector.Claude{}
	p := "  Read(/some/very/long/path/to/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. Yes, allow reading from src/ during this session\n   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Error("should detect when separator scrolled off (fallback top)")
	}
}

func TestClaude_SeparatorScrolledOffNoEsc(t *testing.T) {
	d := detector.Claude{}
	p := "  Read(/some/very/long/path/to/file.rs)\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n   2. No\n"
	if d.Detect(p) == nil {
		t.Error("should detect with both fallback boundaries")
	}
}

func TestClaude_SeparatorScrolledOffEditVariant(t *testing.T) {
	d := detector.Claude{}
	p := "  some long content\n\n" +
		" Do you want to edit the file?\n" +
		"   1. Yes\n   2. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	if d.Detect(p) == nil {
		t.Error("should detect 'Do you want to edit' variant")
	}
}

func TestClaude_SeparatorPreferredOverFallback(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Read file\n\n Do you want to proceed?\n" +
		"   1. Yes\n   2. No\n\n Esc to cancel\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestClaude_DefaultResponse(t *testing.T) {
	d := detector.Claude{}
	p := claudePrompt("Read file", []string{"Yes", "No"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want %q", r.Response, "\r")
	}
}

func TestClaude_DontAskAgainSelectsOption2(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Bash command\n\n" +
		"   python3 -c \"import json\"\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n" +
		"   2. Yes, and don't ask again for: python3 *\n" +
		"   3. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\x1b[B\r" {
		t.Errorf("Response = %q, want \"\\x1b[B\\r\" (↓+Enter)", r.Response)
	}
}

func TestClaude_2OptionNoAskAgainKeepsEnter(t *testing.T) {
	d := detector.Claude{}
	p := "─────────────────────────────────────────────\n" +
		" Bash command\n\n" +
		"   ls -la\n\n" +
		" Contains simple_expansion\n\n" +
		" Do you want to proceed?\n" +
		"   1. Yes\n" +
		"   2. No\n\n" +
		" Esc to cancel · Tab to amend\n"
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \"\\r\"", r.Response)
	}
}
