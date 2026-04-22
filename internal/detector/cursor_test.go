// internal/detector/cursor_test.go
package detector_test

import (
	"strings"
	"testing"

	"github.com/host452b/yoyo/internal/detector"
)

func cursorBox(lines []string) string {
	width := 60
	top := "┌" + strings.Repeat("─", width) + "┐\n"
	bottom := "└" + strings.Repeat("─", width) + "┘\n"
	var sb strings.Builder
	sb.WriteString(top)
	for _, line := range lines {
		sb.WriteString("│ ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(bottom)
	return sb.String()
}

func TestCursor_CommandPrompt(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{
		"Run this command?",
		"Not in allowlist: cargo test",
		" → Run (once) (y)",
		"   Skip (esc or n)",
	})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.RuleName != "Cursor" {
		t.Errorf("RuleName = %q, want 'Cursor'", r.RuleName)
	}
}

func TestCursor_NoMatchWithoutOptions(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect(cursorBox([]string{"Some text without options"})) != nil {
		t.Error("should not detect without (y)/(n) options")
	}
}

func TestCursor_NoMatchWithoutBox(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect("Run (once) (y)\nSkip (esc or n)") != nil {
		t.Error("should not detect without box drawing")
	}
}

func TestCursor_IgnoresInputBox(t *testing.T) {
	d := detector.Cursor{}
	if d.Detect(cursorBox([]string{"→ Plan, search, build anything"})) != nil {
		t.Error("should ignore input box without (y)/(n)")
	}
}

func TestCursor_PicksLastBox(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{"→ Plan, search, build anything"}) +
		cursorBox([]string{"Run this command?", " → Run (once) (y)", "   Skip (esc or n)"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
}

func TestCursor_DefaultResponse(t *testing.T) {
	d := detector.Cursor{}
	p := cursorBox([]string{"Run (once) (y)", "Skip (esc or n)"})
	r := d.Detect(p)
	if r == nil {
		t.Fatal("expected detection")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \\r", r.Response)
	}
}

// TestCursor_PromptBelowCommandBox covers the layout where the command sits
// inside the box and the approval question/options render below it.
func TestCursor_PromptBelowCommandBox(t *testing.T) {
	d := detector.Cursor{}
	screen := cursorBox([]string{"$  lspci -nnk -s 01:00.0 in ."}) +
		"\n" +
		"  Run this command?\n" +
		"  Not in allowlist: lspci -nnk -s 01:00.0\n" +
		"   → Run (once) (y)\n" +
		"     Skip (esc or n)\n"
	r := d.Detect(screen)
	if r == nil {
		t.Fatal("expected detection for prompt rendered below command box")
	}
	if r.Response != "\r" {
		t.Errorf("Response = %q, want \\r", r.Response)
	}
}

// TestCursor_CommandBoxWithoutPromptIsIgnored guards against false positives:
// a command box with no approval UI below it must not match.
func TestCursor_CommandBoxWithoutPromptIsIgnored(t *testing.T) {
	d := detector.Cursor{}
	screen := cursorBox([]string{"$ ls"}) + "\nsome output line\nanother line\n"
	if d.Detect(screen) != nil {
		t.Error("should not detect without (y)/(n) markers below box")
	}
}
