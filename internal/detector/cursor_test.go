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
